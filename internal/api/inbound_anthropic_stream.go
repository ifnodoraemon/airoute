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

	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		statusCode, errType, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondAnthropicError(c, statusCode, sanitizeErrorMessage(err), errType)
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
	accumulatedCompChars := 0
	upstreamUsageSeen := false
	usageSource := "upstream"
	var streamError error

	// finalizeCompletion falls back to a character-based estimate when the
	// upstream stream never reported usage, instead of counting SSE deltas.
	finalizeCompletion := func() {
		if upstreamUsageSeen {
			return
		}
		if accumulatedCompChars > 0 {
			totalCompTokens = accumulatedCompChars / 3
			if totalCompTokens < 1 {
				totalCompTokens = 1
			}
		}
		usageSource = "estimated"
	}
	upstreamChannel := ""         // populated from the first stream event
	anthropicUpstreamChatID := "" // upstream response ID, captured from the first chunk
	currentBlockIndex := 0
	textBlockOpened := true
	toolBlockOpened := false
	stopReason := "end_turn"

	// Audit record: persisted exactly once on every exit path — normal end,
	// upstream mid-stream failure, or downstream client disconnect.
	var finalCost, finalSaved float64
	var finalCached int
	var recordOnce sync.Once
	recordStreamEnd := func(statusCode int) {
		// A client abort still consumed the prompt upstream: the request had been
		// dispatched before the disconnect, so bill the estimated prompt instead
		// of recording zero.
		if statusCode == http.StatusOK || statusCode == statusClientClosed {
			// Anthropic wire format requires input_tokens on every message. When
			// the upstream stream omits usage, approximate the prompt from the
			// request so the audit log does not show a misleading zero.
			if totalPromptTokens == 0 {
				if estimated := estimateCanonicalPromptTokens(canonicalReq); estimated > 0 {
					totalPromptTokens = estimated
					usageSource = "estimated"
				}
			}
			if totalCompTokens == 0 && accumulatedCompChars > 0 {
				totalCompTokens = estimateTokens(accumulatedCompChars)
				usageSource = "estimated"
			}
		}
		finalCost, finalSaved, finalCached = recordUsage(c, AuditRecordParams{
			SessionID:        sessionID,
			ChatID:           anthropicUpstreamChatID,
			Channel:          upstreamChannel,
			Model:            req.Model,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			CachedTokens:     totalCachedTokens,
			Duration:         time.Since(start),
			TTFT:             ttftDuration,
			StatusCode:       statusCode,
			UsageSource:      usageSource,
			LiveTracked:      true, // stream deltas already fed the live token meter
			ErrorMessage:     sanitizeErrorMessage(streamError),
		})
	}
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

	// 1. Emit event: message_start
	startEvent := gin.H{
		"type": "message_start",
		"message": gin.H{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         req.Model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": gin.H{
				"input_tokens":  totalPromptTokens,
				"output_tokens": 1,
			},
		},
	}
	startBytes, _ := json.Marshal(startEvent)
	fmt.Fprintf(c.Writer, "event: message_start\ndata: %s\n\n", startBytes)

	// 2. Emit event: content_block_start
	blockStart := gin.H{
		"type":  "content_block_start",
		"index": 0,
		"content_block": gin.H{
			"type": "text",
			"text": "",
		},
	}
	blockStartBytes, _ := json.Marshal(blockStart)
	fmt.Fprintf(c.Writer, "event: content_block_start\ndata: %s\n\n", blockStartBytes)
	flusher.Flush()

	w := c.Writer
	pending := firstEvent
	for {
		var event *model.StreamEvent
		var open bool
		if pending != nil {
			event, open = pending, true
			pending = nil
		} else {
			select {
			case <-c.Request.Context().Done():
				return
			case event, open = <-streamChan:
			}
		}
		{
			if !open {
				// Stream finished
				finalizeCompletion()
				recordOnce.Do(func() { recordStreamEnd(http.StatusOK) })
				if textBlockOpened || toolBlockOpened {
					fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", currentBlockIndex)
				}

				// Emit message_delta
				deltaEvent := gin.H{
					"type": "message_delta",
					"delta": gin.H{
						"stop_reason":   stopReason,
						"stop_sequence": nil,
					},
					"usage": gin.H{
						"input_tokens":            max(0, totalPromptTokens-totalCachedTokens),
						"cache_read_input_tokens": totalCachedTokens,
						"output_tokens":           totalCompTokens,
					},
				}
				deltaBytes, _ := json.Marshal(deltaEvent)
				fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", deltaBytes)

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

						blockDelta := gin.H{
							"type":  "content_block_delta",
							"index": 0,
							"delta": gin.H{
								"type": "text_delta",
								"text": chunkDelta.Content,
							},
						}
						deltaBytes, _ := json.Marshal(blockDelta)
						fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", deltaBytes)
						flusher.Flush()
					}

					if len(chunkDelta.ToolCalls) > 0 {
						for _, tc := range chunkDelta.ToolCalls {
							if tc.ID != "" && tc.Function.Name != "" {
								if textBlockOpened {
									fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", currentBlockIndex)
									textBlockOpened = false
								}
								if toolBlockOpened {
									fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", currentBlockIndex)
								}
								currentBlockIndex++
								toolBlockOpened = true
								stopReason = "tool_use"

								tbStart := gin.H{
									"type":  "content_block_start",
									"index": currentBlockIndex,
									"content_block": gin.H{
										"type": "tool_use",
										"id":   tc.ID,
										"name": tc.Function.Name,
									},
								}
								tbBytes, _ := json.Marshal(tbStart)
								fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", tbBytes)
								flusher.Flush()
							}

							if tc.Function.Arguments != "" {
								stopReason = "tool_use"
								argDelta := gin.H{
									"type":  "content_block_delta",
									"index": currentBlockIndex,
									"delta": gin.H{
										"type":         "input_json_delta",
										"partial_json": tc.Function.Arguments,
									},
								}
								argBytes, _ := json.Marshal(argDelta)
								fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", argBytes)
								flusher.Flush()
							}
						}
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
				}
			}
		}
	}
}

// Native Anthropic streams retain signed thinking blocks, block ordering, and
// provider-specific SSE fields while still flowing through dispatcher fallback.
func (h *Handler) handleNativeAnthropicStream(c *gin.Context, stream <-chan *model.StreamEvent, first *model.StreamEvent, modelName, sessionID string, start time.Time) {
	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondAnthropicError(c, http.StatusInternalServerError, "Streaming unsupported", "api_error")
		return
	}
	inputTokens, outputTokens := 0, 0
	cachedReadTokens, cacheCreationTokens := 0, 0
	var ttftDuration time.Duration
	var firstContentSeen bool
	chatID, channel, statusCode := "", first.Channel, http.StatusOK
	var streamErr error
	defer func() {
		// Anthropic input_tokens excludes prompt-cache reads/writes; the audit
		// record must report the full prompt and surface cache hits separately.
		totalPrompt := inputTokens + cachedReadTokens + cacheCreationTokens
		RecordUsage(c, AuditRecordParams{
			SessionID:        sessionID,
			ChatID:           chatID,
			Channel:          channel,
			Model:            modelName,
			PromptTokens:     totalPrompt,
			CompletionTokens: outputTokens,
			CachedTokens:     cachedReadTokens,
			Duration:         time.Since(start),
			TTFT:             ttftDuration,
			StatusCode:       statusCode,
			UsageSource:      "upstream",
			ErrorMessage:     sanitizeErrorMessage(streamErr),
			LiveTracked:      true, // stream deltas already fed the live token meter
		})
	}()
	write := func(event *model.StreamEvent) bool {
		if event == nil {
			return true
		}
		if event.Channel != "" {
			channel = event.Channel
		}
		if event.Err != nil {
			streamErr = event.Err
			statusCode = http.StatusBadGateway
			body, _ := json.Marshal(gin.H{"type": "error", "error": gin.H{"type": "stream_error", "message": sanitizeErrorMessage(event.Err)}})
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", body)
			flusher.Flush()
			return false
		}
		if len(event.NativeAnthropicEvent) == 0 {
			return true
		}
		var payload map[string]any
		if json.Unmarshal(event.NativeAnthropicEvent, &payload) == nil {
			if !firstContentSeen && (event.NativeAnthropicType == "content_block_delta" || event.NativeAnthropicType == "content_block_start") {
				ttftDuration = time.Since(start)
				telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
				firstContentSeen = true
			}
			if event.NativeAnthropicType == "message_start" {
				if msg, ok := payload["message"].(map[string]any); ok {
					chatID, _ = msg["id"].(string)
					msg["model"] = modelName
					if usage, ok := msg["usage"].(map[string]any); ok {
						inputTokens = intFromJSONNumber(usage["input_tokens"])
						cachedReadTokens = intFromJSONNumber(usage["cache_read_input_tokens"])
						cacheCreationTokens = intFromJSONNumber(usage["cache_creation_input_tokens"])
					}
				}
			} else if event.NativeAnthropicType == "content_block_delta" {
				// 实时吞吐：从原生 delta 中取出文本增量计入内存 token 计数。
				if delta, ok := payload["delta"].(map[string]any); ok {
					if text, ok := delta["text"].(string); ok && text != "" {
						telemetry.GlobalLiveMetrics.AddChars(len(text))
					}
				}
			} else if event.NativeAnthropicType == "message_delta" {
				if usage, ok := payload["usage"].(map[string]any); ok {
					outputTokens = intFromJSONNumber(usage["output_tokens"])
					if value, exists := usage["input_tokens"]; exists {
						inputTokens = intFromJSONNumber(value)
					}
					if value, exists := usage["cache_read_input_tokens"]; exists {
						cachedReadTokens = intFromJSONNumber(value)
					}
					if value, exists := usage["cache_creation_input_tokens"]; exists {
						cacheCreationTokens = intFromJSONNumber(value)
					}
				}
			}
			event.NativeAnthropicEvent, _ = json.Marshal(payload)
		}
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.NativeAnthropicType, event.NativeAnthropicEvent)
		flusher.Flush()
		return true
	}
	if !write(first) {
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			// 499 matches the other stream handlers: the *client* went away, the
			// upstream did not time out.
			statusCode = 499
			return
		case event, open := <-stream:
			if !open {
				return
			}
			if !write(event) {
				return
			}
		}
	}
}

func intFromJSONNumber(value any) int {
	if number, ok := value.(float64); ok {
		return int(number)
	}
	return 0
}
