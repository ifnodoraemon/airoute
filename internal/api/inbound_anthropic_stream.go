package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// handleAnthropicStream handles SSE streaming responses for Claude SDK clients.
func (h *Handler) handleAnthropicStream(c *gin.Context, reqCtx context.Context, req *AnthropicInboundRequest, canonicalReq *model.ChatCompletionRequest, sessionID string, start time.Time) {
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		statusCode := http.StatusBadGateway
		errType := "api_error"
		if strings.Contains(err.Error(), "no upstream provider available") {
			statusCode = http.StatusNotFound
			errType = "not_found_error"
		}
		recordFailedRequest(c, sessionID, canonicalReq.Model, time.Since(start), statusCode)
		RespondAnthropicError(c, statusCode, err.Error(), errType)
		return
	}

	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondAnthropicError(c, http.StatusInternalServerError, "Streaming unsupported", "api_error")
		return
	}

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	firstTokenRecorded := false
	totalPromptTokens := 0
	totalCompTokens := 0
	upstreamChannel := ""         // populated from the first stream event
	anthropicUpstreamChatID := "" // upstream response ID, captured from the first chunk
	currentBlockIndex := 0
	textBlockOpened := true
	toolBlockOpened := false
	stopReason := "end_turn"

	// Audit record: persisted exactly once on every exit path — normal end,
	// upstream mid-stream failure, or downstream client disconnect.
	var recordOnce sync.Once
	recordStreamEnd := func() {
		RecordUsage(c, AuditRecordParams{
			SessionID:        sessionID,
			ChatID:           anthropicUpstreamChatID,
			Channel:          upstreamChannel,
			Model:            req.Model,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			Duration:         time.Since(start),
			StatusCode:       http.StatusOK,
		})
	}
	defer recordOnce.Do(recordStreamEnd)

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
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-streamChan:
			if !open {
				// Stream finished
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
						"output_tokens": totalCompTokens,
					},
				}
				deltaBytes, _ := json.Marshal(deltaEvent)
				fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", deltaBytes)

				// Emit message_stop
				fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				flusher.Flush()

				recordOnce.Do(recordStreamEnd)
				return
			}

			// Capture the serving channel before any short-circuit (terminal
			// events return/continue first); the dispatcher stamps every event.
			if upstreamChannel == "" && event.Channel != "" {
				upstreamChannel = event.Channel
			}

			if event.Err != nil {
				errBytes, _ := json.Marshal(gin.H{
					"type": "error",
					"error": gin.H{
						"type":    "stream_error",
						"message": event.Err.Error(),
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
							telemetry.GlobalMetrics.RecordTTFT(time.Since(start))
							firstTokenRecorded = true
						}
						totalCompTokens++

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

					if event.Chunk.Usage != nil {
						totalPromptTokens = event.Chunk.Usage.PromptTokens
						totalCompTokens = event.Chunk.Usage.CompletionTokens
					}
				}
			}
		}
	}
}
