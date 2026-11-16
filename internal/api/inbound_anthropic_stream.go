package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// handleAnthropicStream handles SSE streaming responses for Claude SDK clients.
func (h *Handler) handleAnthropicStream(c *gin.Context, reqCtx context.Context, req *AnthropicInboundRequest, canonicalReq *model.ChatCompletionRequest, sessionID string, start time.Time) {
	// Track the in-flight stream for the dashboard's live telemetry gauges. This
	// covers both the adapted and the native Anthropic stream paths below.
	telemetry.GlobalLiveMetrics.BeginStream()
	defer telemetry.GlobalLiveMetrics.EndStream()

	// Ensure usage is requested from OpenAI-compatible upstreams regardless of
	// how the canonical request was built.
	if canonicalReq.StreamOptions == nil {
		canonicalReq.StreamOptions = &model.StreamOptions{IncludeUsage: true}
	} else {
		canonicalReq.StreamOptions.IncludeUsage = true
	}

	if !topUpQuotaForRequest(c, canonicalReq) {

		return

	}
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		statusCode, errType, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondAnthropicError(c, statusCode, sanitizeErrorMessage(err), errType)
		return
	}
	var nativeHandled bool
	streamChan, nativeHandled = h.useNativeStream(c, reqCtx, canonicalReq, streamChan, sessionID, start)
	if nativeHandled {
		return
	}

	firstEvent, open := <-streamChan
	if !open || firstEvent == nil {
		streamErr := errors.New("upstream stream closed before response")
		recordFailedRequestWithError(c, sessionID, req.Model, time.Since(start), http.StatusBadGateway, streamErr)
		RespondAnthropicError(c, http.StatusBadGateway, "upstream stream closed before response", "api_error")
		return
	}
	if len(firstEvent.NativeAnthropicEvent) > 0 {
		h.handleNativeAnthropicStream(c, streamChan, firstEvent, req.Model, sessionID, start)
		return
	}

	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondAnthropicError(c, http.StatusInternalServerError, "Streaming unsupported", "api_error")
		return
	}

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixMicro())
	firstTokenRecorded := false
	var ttftDuration time.Duration
	totalPromptTokens := 0
	totalCompTokens := 0
	totalCachedTokens := 0
	totalCacheCreationTokens := 0
	accumulatedCompChars := 0
	upstreamUsageSeen := false
	usageSource := "upstream"
	var streamError error

	// finalizeCompletion falls back to a character-based estimate when the
	// upstream stream never reported usage, instead of counting SSE deltas.
	// The estimate goes through the shared estimator: this endpoint used to
	// divide by 3 while every other path divided by 4, so the same output was
	// billed one third higher here.
	finalizeCompletion := func() {
		if upstreamUsageSeen {
			return
		}
		if accumulatedCompChars > 0 {
			totalCompTokens = estimatedStreamCompletionTokens(accumulatedCompChars)
		}
		usageSource = "estimated"
	}
	upstreamChannel := ""         // populated from the first stream event
	anthropicUpstreamChatID := "" // upstream response ID, captured from the first chunk
	currentBlockIndex := 0
	textBlockOpened := true
	// OpenAI may interleave parallel tool_call fragments by index, but Anthropic
	// content blocks are strictly sequential. Assemble the calls first and emit
	// them in first-seen order at finalization rather than streaming each fragment
	// onto whichever block happens to be open.
	tools := newGeminiStreamToolCalls()
	stopReason := "end_turn"

	// Audit record: persisted exactly once on every exit path — normal end,
	// upstream mid-stream failure, or downstream client disconnect.
	var finalCost, finalSaved float64
	var finalCached int
	// Settlement is deferred until the terminal frame has been flushed, so a
	// saturated settlement writer cannot delay [DONE]. Preparation (cost
	// calculation, headers, audit enqueue) already happened in recordStreamEnd.
	var finalSettle func()
	settled := false
	runFinalSettle := func() {
		if settled {
			return
		}
		settled = true
		if finalSettle != nil {
			finalSettle()
		}
	}
	var recordOnce sync.Once
	recordStreamEnd := func(statusCode int) {
		finalCost, finalSaved, finalCached, finalSettle = recordAnthropicStreamEnd(c, statusCode, &totalPromptTokens, &totalCompTokens, &usageSource, accumulatedCompChars, canonicalReq, AuditRecordParams{
			SessionID:           sessionID,
			ChatID:              anthropicUpstreamChatID,
			Channel:             upstreamChannel,
			Model:               req.Model,
			CachedTokens:        totalCachedTokens,
			CacheCreationTokens: totalCacheCreationTokens,
			Duration:            time.Since(start),
			TTFT:                ttftDuration,
			StatusCode:          statusCode,
			LiveTracked:         true, // stream deltas already fed the live token meter
			ErrorMessage:        sanitizeErrorMessage(streamError),
		})
	}
	defer runFinalSettle()
	defer func() {
		// Failure or client disconnect: record once with a truthful status.
		recordOnce.Do(func() {
			statusCode := http.StatusOK
			if streamError != nil {
				statusCode = http.StatusBadGateway
			}
			if c.Request.Context().Err() != nil {
				statusCode = 499
			}
			recordStreamEnd(statusCode)
		})
	}()

	// 1-2. Emit message_start and the initial text content_block_start.
	emitAnthropicStreamStart(c.Writer, flusher, msgID, req.Model, totalPromptTokens)

	w := c.Writer
	pending := firstEvent
	clientGone := false
	drainEnabled := upstreamDrainingEnabled()
	doneCh := c.Request.Context().Done()
	for {
		var event *model.StreamEvent
		var open bool
		if pending != nil {
			event, open = pending, true
			pending = nil
		} else {
			select {
			case <-doneCh:
				if !drainEnabled {
					return
				}
				// Customer left: keep consuming the provider stream so its usage
				// report still arrives and the charge matches the invoice.
				clientGone = true
				doneCh = nil
				markAbortedDrain(0)
				continue
			case event, open = <-streamChan:
			}
		}
		{
			if !open {
				// Stream finished
				finalizeCompletion()
				finalStatus := http.StatusOK
				if clientGone {
					finalStatus = statusClientClosed
					markAbortedDrain(totalCompTokens)
				}
				recordOnce.Do(func() { recordStreamEnd(finalStatus) })
				if clientGone {
					return
				}
				if textBlockOpened {
					fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", currentBlockIndex)
					textBlockOpened = false
				}
				currentBlockIndex = emitAnthropicToolBlocks(w, flusher, tools, currentBlockIndex)

				// Emit message_delta
				emitAnthropicMessageDelta(w, flusher, stopReason, totalPromptTokens, totalCachedTokens, totalCompTokens)

				recordOnce.Do(func() { recordStreamEnd(http.StatusOK) })
				writeStreamUsageTrailer(w, flusher, finalCost, finalSaved, finalCached)

				// Emit message_stop last so clients see a clean terminator.
				fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				flusher.Flush()
				return
			}

			// Capture the serving channel before any short-circuit (terminal
			// events return/continue first); the dispatcher stamps every event.
			if upstreamChannel == "" && event.Channel != "" {
				upstreamChannel = event.Channel
			}

			if event.Err != nil {
				streamError = event.Err
				errBytes, _ := json.Marshal(gin.H{
					"type": "error",
					"error": gin.H{
						"type":    "stream_error",
						"message": sanitizeErrorMessage(event.Err),
					},
				})
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", errBytes)
				flusher.Flush()
				return
			}

			if event.IsDone {
				continue
			}

			if event.Chunk != nil {
				// Capture the upstream response ID from the first chunk
				if event.Chunk.ID != "" && anthropicUpstreamChatID == "" {
					anthropicUpstreamChatID = event.Chunk.ID
				}

				if len(event.Chunk.Choices) > 0 {
					chunkChoice := event.Chunk.Choices[0]
					chunkDelta := chunkChoice.Delta

					if chunkChoice.FinishReason != nil {
						if *chunkChoice.FinishReason == "tool_calls" {
							stopReason = "tool_use"
						} else if *chunkChoice.FinishReason == "length" {
							stopReason = "max_tokens"
						} else if *chunkChoice.FinishReason == "content_filter" {
							stopReason = "refusal"
						}
					}

					if chunkDelta.Content != "" {
						if !firstTokenRecorded {
							ttftDuration = time.Since(start)
							telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
							firstTokenRecorded = true
						}
						accumulatedCompChars += len(chunkDelta.Content)
						// 实时吞吐：流式 delta 到达即计入内存 token 计数。
						telemetry.GlobalLiveMetrics.AddChars(len(chunkDelta.Content))
						emitAnthropicTextDelta(w, flusher, chunkDelta.Content, &currentBlockIndex, &textBlockOpened)
					}

					if len(chunkDelta.ToolCalls) > 0 {
						for _, tc := range chunkDelta.ToolCalls {
							tools.add(tc)
						}
						stopReason = "tool_use"
					}

				}

				// Usage can arrive on a frame whose choices array is empty, so it
				// must be captured outside the choice-handling block.
				if event.Chunk.Usage != nil {
					if event.Chunk.Usage.PromptTokens > 0 || event.Chunk.Usage.CompletionTokens > 0 {
						usageSource = "upstream"
						upstreamUsageSeen = true
					}
					totalPromptTokens = max(totalPromptTokens, event.Chunk.Usage.PromptTokens)
					totalCompTokens = max(totalCompTokens, event.Chunk.Usage.CompletionTokens)
					totalCachedTokens = max(totalCachedTokens, event.Chunk.Usage.GetCachedTokens())
					totalCacheCreationTokens = max(totalCacheCreationTokens, event.Chunk.Usage.GetCacheCreationTokens())
				}
			}
		}
	}
}
