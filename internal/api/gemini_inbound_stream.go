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
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		recordFailedRequest(c, sessionID, canonicalReq.Model, time.Since(start), http.StatusBadGateway)
		RespondGeminiError(c, http.StatusBadGateway, err.Error(), "UNAVAILABLE")
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
	upstreamChannel := ""      // populated from the first stream event
	geminiUpstreamChatID := "" // upstream response ID, captured from the first chunk

	var recordOnce sync.Once
	recordStreamEnd := func() {
		RecordUsage(c, AuditRecordParams{
			SessionID:        sessionID,
			ChatID:           geminiUpstreamChatID,
			Channel:          upstreamChannel,
			Model:            modelName,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			Duration:         time.Since(start),
			StatusCode:       http.StatusOK,
		})
	}
	defer recordOnce.Do(recordStreamEnd)

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-streamChan:
			if !open {
				recordOnce.Do(recordStreamEnd)
				return
			}

			// Capture the serving channel before any short-circuit (terminal
			// events return/continue first); the dispatcher stamps every event.
			if upstreamChannel == "" && event.Channel != "" {
				upstreamChannel = event.Channel
			}

			if event.Err != nil {
				errChunk := gin.H{
					"error": gin.H{
						"code":    500,
						"message": event.Err.Error(),
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

				if len(event.Chunk.Choices) > 0 {
					chunkChoice := event.Chunk.Choices[0]
					var geminiParts []gin.H

					if chunkChoice.Delta.Content != "" {
						if !firstTokenRecorded {
							telemetry.GlobalMetrics.RecordTTFT(time.Since(start))
							firstTokenRecorded = true
						}
						totalCompTokens++
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
							totalPromptTokens = event.Chunk.Usage.PromptTokens
							totalCompTokens = event.Chunk.Usage.CompletionTokens
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
