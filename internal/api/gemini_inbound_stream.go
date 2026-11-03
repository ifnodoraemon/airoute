package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// handleGeminiStream handles streaming SSE responses for Google Gemini SDK clients.
func (h *Handler) handleGeminiStream(c *gin.Context, reqCtx context.Context, canonicalReq *model.ChatCompletionRequest, modelName, sessionID string, start time.Time) {
	// Track the in-flight stream for the dashboard's live telemetry gauges.
	telemetry.GlobalLiveMetrics.BeginStream()
	defer telemetry.GlobalLiveMetrics.EndStream()

	topUpQuotaForRequest(c, canonicalReq)
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		statusCode, _, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondGeminiError(c, statusCode, sanitizeErrorMessage(err), geminiErrorStatus(statusCode))
		return
	}

	var nativeHandled bool
	streamChan, nativeHandled = h.useNativeStream(c, reqCtx, canonicalReq, streamChan, sessionID, start)
	if nativeHandled {
		return
	}

	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondGeminiError(c, http.StatusInternalServerError, "Streaming unsupported", "INTERNAL")
		return
	}

	w := c.Writer
	totalPromptTokens := 0
	totalCompTokens := 0
	totalCachedTokens := 0
	totalThoughtTokens := 0
	totalCacheCreationTokens := 0
	firstTokenRecorded := false
	var ttftDuration time.Duration
	upstreamChannel := ""      // populated from the first stream event
	geminiUpstreamChatID := "" // upstream response ID, captured from the first chunk

	accumulatedChars := 0
	usageSource := "upstream"
	var streamError error

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
		// A client abort (499) still sent the prompt upstream; estimate it too.
		if statusCode == http.StatusOK || statusCode == statusClientClosed {
			canonicalEstimate := estimateCanonicalPromptTokens(canonicalReq)
			if totalPromptTokens == 0 {
				totalPromptTokens = streamFallbackPromptTokens(canonicalReq)
				usageSource = "estimated"
			} else {
				observePromptEstimate(canonicalReq.Model, canonicalEstimate, totalPromptTokens)
			}
			if totalCompTokens == 0 && accumulatedChars > 0 {
				totalCompTokens = estimatedStreamCompletionTokens(accumulatedChars)
				usageSource = "estimated"
			}
		}
		_, _, _, finalSettle = RecordUsageDeferred(c, AuditRecordParams{
			SessionID:           sessionID,
			ChatID:              geminiUpstreamChatID,
			Channel:             upstreamChannel,
			Model:               modelName,
			PromptTokens:        totalPromptTokens,
			CompletionTokens:    totalCompTokens,
			CachedTokens:        totalCachedTokens,
			CacheCreationTokens: totalCacheCreationTokens,
			Duration:            time.Since(start),
			TTFT:                ttftDuration,
			StatusCode:          statusCode,
			UsageSource:         usageSource,
			LiveTracked:         true, // stream deltas already fed the live token meter
			ErrorMessage:        sanitizeErrorMessage(streamError),
		})
	}
	defer runFinalSettle()
	defer func() {
		recordOnce.Do(func() {
			statusCode := http.StatusOK
			if streamError != nil {
				statusCode = http.StatusBadGateway
			}
			recordStreamEnd(statusCode)
		})
	}()

	// BUG-GEMINI-TOOLCALL: OpenAI streams a tool call as fragments (index-keyed,
	// arguments split across frames), while a Gemini functionCall part must carry
	// the complete name and a complete args object. Accumulate the fragments and
	// emit one functionCall per call — on the terminal frame, or when the stream
	// ends without one.
	toolsByCandidate := make(map[int]*geminiStreamToolCalls)
	toolCallsFor := func(index int) *geminiStreamToolCalls {
		if toolsByCandidate[index] == nil {
			toolsByCandidate[index] = newGeminiStreamToolCalls()
		}
		return toolsByCandidate[index]
	}

	emitCandidate := func(index int, parts []gin.H, finishReason string, withUsage bool) {
		var usage gin.H
		if withUsage {
			usage = geminiStreamUsage(totalPromptTokens, totalCompTokens, totalCachedTokens, totalThoughtTokens)
		}
		writeGeminiCandidate(w, flusher, index, parts, finishReason, usage)
	}

	clientGone := false
	drainEnabled := upstreamDrainingEnabled()
	doneCh := c.Request.Context().Done()
	for {
		select {
		case <-doneCh:
			if !drainEnabled {
				return
			}
			// Customer left: keep reading so the provider's usage report arrives.
			clientGone = true
			doneCh = nil
			markAbortedDrain(0)
		case event, open := <-streamChan:
			if !open {
				// A tool call that never got its terminal frame must still reach the
				// client as one complete functionCall, not be silently dropped.
				for index, calls := range toolsByCandidate {
					if calls.pending() {
						emitCandidate(index, geminiFunctionCallParts(calls.drainFunctionCalls()), "", false)
					}
				}
				finalStatus := http.StatusOK
				if clientGone {
					finalStatus = statusClientClosed
					markAbortedDrain(totalCompTokens)
				}
				recordOnce.Do(func() { recordStreamEnd(finalStatus) })
				return
			}

			// Capture the serving channel before any short-circuit (terminal
			// events return/continue first); the dispatcher stamps every event.
			if upstreamChannel == "" && event.Channel != "" {
				upstreamChannel = event.Channel
			}

			if event.Err != nil {
				streamError = event.Err
				errChunk := gin.H{
					"error": gin.H{
						"code":    500,
						"message": sanitizeErrorMessage(event.Err),
					},
				}
				errBytes, _ := json.Marshal(errChunk)
				fmt.Fprintf(w, "data: %s\n\n", errBytes)
				flusher.Flush()
				return
			}

			if event.IsDone {
				for index, calls := range toolsByCandidate {
					if calls.pending() {
						emitCandidate(index, geminiFunctionCallParts(calls.drainFunctionCalls()), "", false)
					}
				}
				continue
			}

			if event.Chunk != nil {
				// Capture the upstream response ID from the first chunk
				if event.Chunk.ID != "" && geminiUpstreamChatID == "" {
					geminiUpstreamChatID = event.Chunk.ID
				}

				// Usage may arrive on a frame with an empty choices array; capture it
				// before the choice-specific handling so prompt tokens are not lost.
				usageOnly := len(event.Chunk.Choices) == 0 && event.Chunk.Usage != nil
				if event.Chunk.Usage != nil {
					// Usage counters are cumulative; a frame that reports only the
					// completion side (or only a delta) must not erase input/cache
					// counts an earlier frame already reported. Same rule as the chat
					// and messages streaming handlers.
					totalPromptTokens = max(totalPromptTokens, event.Chunk.Usage.PromptTokens)
					totalCompTokens = max(totalCompTokens, event.Chunk.Usage.CompletionTokens)
					// Cache reads are part of prompt_tokens but bill at the cache
					// rate; not forwarding them charged cached tokens as fresh input.
					totalCachedTokens = max(totalCachedTokens, event.Chunk.Usage.GetCachedTokens())
					totalThoughtTokens = max(totalThoughtTokens, event.Chunk.Usage.GetReasoningTokens())
					totalCacheCreationTokens = max(totalCacheCreationTokens, event.Chunk.Usage.GetCacheCreationTokens())
				}
				if usageOnly {
					usageChunk := gin.H{"usageMetadata": geminiStreamUsage(totalPromptTokens, totalCompTokens, totalCachedTokens, totalThoughtTokens)}
					if b, err := json.Marshal(usageChunk); err == nil {
						fmt.Fprintf(w, "data: %s\n\n", b)
						flusher.Flush()
					}
				}

				for _, chunkChoice := range event.Chunk.Choices {
					toolCalls := toolCallsFor(chunkChoice.Index)
					var geminiParts []gin.H
					if thought := chunkChoice.Delta.ReasoningContent; thought != "" {
						geminiParts = append(geminiParts, gin.H{"text": thought, "thought": true})
						accumulatedChars += len(thought)
						telemetry.GlobalLiveMetrics.AddChars(len(thought))
						if !firstTokenRecorded {
							ttftDuration, firstTokenRecorded = time.Since(start), true
							telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
						}
					}

					if chunkChoice.Delta.Content != "" {
						if !firstTokenRecorded {
							ttftDuration = time.Since(start)
							telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
							firstTokenRecorded = true
						}
						accumulatedChars += len(chunkChoice.Delta.Content)
						// 实时吞吐：流式 delta 到达即计入内存 token 计数。
						telemetry.GlobalLiveMetrics.AddChars(len(chunkChoice.Delta.Content))
						geminiParts = append(geminiParts, gin.H{"text": chunkChoice.Delta.Content})
					}

					// Tool-call fragments are accumulated, not translated one by one:
					// they are emitted as complete functionCall parts on the terminal
					// frame (or at stream end). Argument bytes count toward the
					// completion fallback estimate, exactly like text does.
					if len(chunkChoice.Delta.ToolCalls) > 0 {
						if !firstTokenRecorded {
							ttftDuration = time.Since(start)
							telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
							firstTokenRecorded = true
						}
						for _, tc := range chunkChoice.Delta.ToolCalls {
							toolCalls.add(tc)
							if tc.Function.Arguments != "" {
								accumulatedChars += len(tc.Function.Arguments)
								telemetry.GlobalLiveMetrics.AddChars(len(tc.Function.Arguments))
							}
						}
					}

					finishReason := ""
					if chunkChoice.FinishReason != nil {
						if *chunkChoice.FinishReason == "tool_calls" {
							finishReason = "STOP"
						} else if *chunkChoice.FinishReason == "length" {
							finishReason = "MAX_TOKENS"
						} else if *chunkChoice.FinishReason == "content_filter" {
							finishReason = "SAFETY"
						} else {
							finishReason = "STOP"
						}
					}

					// The terminal frame carries the assembled tool calls, so a client
					// sees each call exactly once with complete arguments.
					if finishReason != "" && toolCalls.pending() {
						geminiParts = append(geminiParts, geminiFunctionCallParts(toolCalls.drainFunctionCalls())...)
					}

					if len(geminiParts) > 0 || finishReason != "" {
						emitCandidate(chunkChoice.Index, geminiParts, finishReason, event.Chunk.Usage != nil)
					}
				}
			}
		}
	}
}
