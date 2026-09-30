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
	"github.com/ifnodoraemon/airoute/internal/adapter"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

// AnthropicInboundMessage represents an inbound message from Claude SDK.
type AnthropicInboundMessage = model.AnthropicInboundMessage

// AnthropicInboundRequest represents the incoming payload from Anthropic SDK.
type AnthropicInboundRequest = model.AnthropicInboundRequest

// extractMessageContent extracts string text from an Anthropic message content field.
func extractMessageContent(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	if blocks, ok := content.([]any); ok {
		var sb strings.Builder
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
		return sb.String()
	}
	b, _ := json.Marshal(content)
	return string(b)
}

var defaultAnthropicAdapter = adapter.NewAnthropicAdapter()

// ConvertAnthropicToCanonical converts an Anthropic request to the canonical OpenAI request format via Adapter Pattern.
func ConvertAnthropicToCanonical(req *AnthropicInboundRequest) *model.ChatCompletionRequest {
	res, err := defaultAnthropicAdapter.ToCanonical(context.Background(), req)
	if err != nil {
		return &model.ChatCompletionRequest{Model: req.Model}
	}
	return res
}

// HandleAnthropicMessages handles POST /v1/messages for Anthropic SDK clients.
func (h *Handler) HandleAnthropicMessages(c *gin.Context) {
	var req AnthropicInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Failed to parse Anthropic request: %v", err),
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "model is required",
			},
		})
		return
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "permission_error",
				"message": fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model),
			},
		})
		return
	}

	// Validate Anthropic protocol and schema rules with optional key config
	valRes := validator.ValidateAnthropicRequest(&req, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		valRes.Error.WriteGinResponse(c)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()
	canonicalReq := ConvertAnthropicToCanonical(&req)

	sessionID := resolveSessionID(c, canonicalReq.Model, canonicalReq.Messages, "")
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	// Non-streaming response for Anthropic clients
	if !req.Stream {
		resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
		if err != nil {
			statusCode := http.StatusBadGateway
			errType := "api_error"
			if strings.Contains(err.Error(), "no upstream provider available") {
				statusCode = http.StatusNotFound
				errType = "not_found_error"
			}
			recordFailedRequest(c, sessionID, canonicalReq.Model, time.Since(start), statusCode)
			c.JSON(statusCode, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    errType,
					"message": err.Error(),
				},
			})
			return
		}

		var contentBlocks []gin.H
		stopReason := "end_turn"

		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			if replyText := choice.Message.GetContentString(); replyText != "" {
				contentBlocks = append(contentBlocks, gin.H{
					"type": "text",
					"text": replyText,
				})
			}
			for _, tc := range choice.Message.ToolCalls {
				var parsedArgs any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsedArgs); err != nil {
					parsedArgs = map[string]any{}
				}
				contentBlocks = append(contentBlocks, gin.H{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": parsedArgs,
				})
			}

			if len(choice.Message.ToolCalls) > 0 || (choice.FinishReason != nil && *choice.FinishReason == "tool_calls") {
				stopReason = "tool_use"
			} else if choice.FinishReason != nil && *choice.FinishReason == "length" {
				stopReason = "max_tokens"
			}
		}

		inputTokens := 0
		outputTokens := 0
		if resp.Usage != nil {
			inputTokens = resp.Usage.PromptTokens
			outputTokens = resp.Usage.CompletionTokens
		}

		anthropicResp := gin.H{
			"id":          fmt.Sprintf("msg_%s", resp.ID),
			"type":        "message",
			"role":        "assistant",
			"model":       req.Model,
			"content":     contentBlocks,
			"stop_reason": stopReason,
			"usage": gin.H{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			},
		}

		var cost, savedCost float64
		keyGroup := getKeyGroup(c)
		if billing.GlobalEngine != nil {
			cost, savedCost = billing.GlobalEngine.CalculateCostWithGroup(req.Model, keyGroup, inputTokens, outputTokens, 0)
		}
		_ = savedCost
		dur := time.Since(start)
		telemetry.GlobalMetrics.RecordRequestWithModel(req.Model, true, dur, inputTokens, outputTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				ChatID:           resp.ID,
				Channel:          resp.Channel,
				SessionID:        sessionID,
				APIKey:           getRequestAPIKey(c),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     inputTokens,
				CompletionTokens: outputTokens,
				TotalTokens:      inputTokens + outputTokens,
				Cost:             cost,
				DurationMs:       time.Since(start).Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}

		c.JSON(http.StatusOK, anthropicResp)
		return
	}

	// Streaming SSE response for Anthropic clients
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		statusCode := http.StatusBadGateway
		errType := "api_error"
		if strings.Contains(err.Error(), "no upstream provider available") {
			statusCode = http.StatusNotFound
			errType = "not_found_error"
		}
		recordFailedRequest(c, sessionID, canonicalReq.Model, time.Since(start), statusCode)
		c.JSON(statusCode, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": err.Error(),
			},
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming unsupported"})
		return
	}
	flusher.Flush()

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
	// upstream mid-stream failure, or downstream client disconnect. Tokens
	// counted so far are what get billed, so they must survive any break.
	var recordOnce sync.Once
	recordStreamEnd := func() {
		dur := time.Since(start)
		var cost, savedCost float64
		keyGroup := getKeyGroup(c)
		if billing.GlobalEngine != nil {
			cost, savedCost = billing.GlobalEngine.CalculateCostWithGroup(req.Model, keyGroup, totalPromptTokens, totalCompTokens, 0)
		}
		_ = savedCost
		telemetry.GlobalMetrics.RecordRequest(true, dur, totalPromptTokens, totalCompTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				ChatID:           anthropicUpstreamChatID,
				Channel:          upstreamChannel,
				SessionID:        sessionID,
				APIKey:           getRequestAPIKey(c),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     totalPromptTokens,
				CompletionTokens: totalCompTokens,
				TotalTokens:      totalPromptTokens + totalCompTokens,
				Cost:             cost,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}
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
