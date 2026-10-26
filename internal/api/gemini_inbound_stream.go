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

	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		statusCode, _, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondGeminiError(c, statusCode, sanitizeErrorMessage(err), "UNAVAILABLE")
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
	firstTokenRecorded := false
	var ttftDuration time.Duration
	upstreamChannel := ""      // populated from the first stream event
	geminiUpstreamChatID := "" // upstream response ID, captured from the first chunk

	accumulatedChars := 0
	usageSource := "upstream"
	var streamError error
	var finalCost, finalSaved float64
	var finalCached int

	var recordOnce sync.Once
	recordStreamEnd := func(statusCode int) {
		// A client abort (499) still sent the prompt upstream; estimate it too.
		if statusCode == http.StatusOK || statusCode == statusClientClosed {
			if totalPromptTokens == 0 {
				totalPromptTokens = calibratedPromptEstimate(canonicalReq.Model, estimateCanonicalPromptTokens(canonicalReq))
				usageSource = "estimated"
			} else {
				observePromptEstimate(canonicalReq.Model, estimateCanonicalPromptTokens(canonicalReq), totalPromptTokens)
			}
			if totalCompTokens == 0 && accumulatedChars > 0 {
				totalCompTokens = estimateTokens(accumulatedChars)
				usageSource = "estimated"
			}
		}
		finalCost, finalSaved, finalCached = recordUsage(c, AuditRecordParams{
			SessionID:        sessionID,
			ChatID:           geminiUpstreamChatID,
			Channel:          upstreamChannel,
			Model:            modelName,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			Duration:         time.Since(start),
			TTFT:             ttftDuration,
			StatusCode:       statusCode,
			UsageSource:      usageSource,
			LiveTracked:      true, // stream deltas already fed the live token meter
			ErrorMessage:     sanitizeErrorMessage(streamError),
		})
	}
	defer func() {
		recordOnce.Do(func() {
			statusCode := http.StatusOK
			if streamError != nil {
				statusCode = http.StatusBadGateway
			}
			recordStreamEnd(statusCode)
		})
	}()

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
				finalStatus := http.StatusOK
				if clientGone {
					finalStatus = statusClientClosed
					markAbortedDrain(totalCompTokens)
				}
				recordOnce.Do(func() { recordStreamEnd(finalStatus) })
				if !clientGone {
					writeStreamUsageTrailer(w, flusher, finalCost, finalSaved, finalCached)
				}
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
					totalPromptTokens = event.Chunk.Usage.PromptTokens
					totalCompTokens = event.Chunk.Usage.CompletionTokens
				}
				if usageOnly {
					usageChunk := gin.H{
						"usageMetadata": gin.H{
							"promptTokenCount":     totalPromptTokens,
							"candidatesTokenCount": totalCompTokens,
							"totalTokenCount":      totalPromptTokens + totalCompTokens,
						},
					}
					if b, err := json.Marshal(usageChunk); err == nil {
						fmt.Fprintf(w, "data: %s\n\n", b)
						flusher.Flush()
					}
				}

				if len(event.Chunk.Choices) > 0 {
					chunkChoice := event.Chunk.Choices[0]
					var geminiParts []gin.H

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

					for _, tc := range chunkChoice.Delta.ToolCalls {
						var args map[string]any
						_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
						geminiParts = append(geminiParts, gin.H{
							"functionCall": gin.H{
								"name": tc.Function.Name,
								"args": args,
							},
						})
					}

					finishReason := ""
					if chunkChoice.FinishReason != nil {
						if *chunkChoice.FinishReason == "tool_calls" {
							finishReason = "STOP"
						} else if *chunkChoice.FinishReason == "length" {
							finishReason = "MAX_TOKENS"
						} else {
							finishReason = "STOP"
						}
					}

					if len(geminiParts) > 0 || finishReason != "" {
						candidate := gin.H{
							"index": 0,
							"content": gin.H{
								"role":  "model",
								"parts": geminiParts,
							},
						}
						if finishReason != "" {
							candidate["finishReason"] = finishReason
						}

						chunkObj := gin.H{
							"candidates": []gin.H{candidate},
						}

						if event.Chunk.Usage != nil {
							chunkObj["usageMetadata"] = gin.H{
								"promptTokenCount":     totalPromptTokens,
								"candidatesTokenCount": totalCompTokens,
								"totalTokenCount":      totalPromptTokens + totalCompTokens,
							}
						}

						chunkBytes, _ := json.Marshal(chunkObj)
						fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
						flusher.Flush()
					}
				}
			}
		}
	}
}
