package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/provider"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

// authenticateGemini checks API key passed via query parameter 'key', header 'x-goog-api-key', or Bearer token.
func authenticateGemini(c *gin.Context) bool {
	cfg := config.GetGlobalConfig()
	if len(cfg.VirtualKeys) == 0 {
		return true // open dev mode
	}

	apiKey := c.Query("key")
	if apiKey == "" {
		apiKey = c.GetHeader("x-goog-api-key")
	}
	if apiKey == "" {
		authH := c.GetHeader("Authorization")
		if strings.HasPrefix(authH, "Bearer ") {
			apiKey = strings.TrimPrefix(authH, "Bearer ")
		}
	}

	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    401,
				"message": "Missing API key. Please pass key via query parameter '?key=...', 'x-goog-api-key', or 'Authorization: Bearer <key>'",
				"status":  "UNAUTHENTICATED",
			},
		})
		return false
	}

	for _, vk := range cfg.VirtualKeys {
		if vk.Key == apiKey {
			c.Set(middleware.ContextKeyTenant, vk.TenantID)
			c.Set(middleware.ContextKeyVirtualKey, vk.Key)
			c.Set(middleware.ContextKeyVirtualKeyConfig, &vk)
			return true
		}
	}

	c.JSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    401,
			"message": "API key not valid. Please pass a valid API key.",
			"status":  "UNAUTHENTICATED",
		},
	})
	return false
}

// HandleGeminiModels handles GET /v1beta/models.
func (h *Handler) HandleGeminiModels(c *gin.Context) {
	if !authenticateGemini(c) {
		return
	}

	models := h.dispatcher.GetAllSupportedModels()
	type geminiModelInfo struct {
		Name                       string   `json:"name"`
		Version                    string   `json:"version"`
		DisplayName                string   `json:"displayName"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	}

	var list []geminiModelInfo
	for _, m := range models {
		list = append(list, geminiModelInfo{
			Name:                       "models/" + m,
			Version:                    "001",
			DisplayName:                m,
			SupportedGenerationMethods: []string{"generateContent", "countTokens", "embedContent"},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"models": list,
	})
}

// HandleGeminiModelDetail handles GET /v1beta/models/*modelAction.
func (h *Handler) HandleGeminiModelDetail(c *gin.Context) {
	if !authenticateGemini(c) {
		return
	}

	param := strings.TrimPrefix(c.Param("modelAction"), "/")
	modelName := strings.TrimPrefix(param, "models/")

	channels := h.dispatcher.GetChannelsForModel(modelName)
	if len(channels) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    404,
				"message": fmt.Sprintf("models/%s is not found", modelName),
				"status":  "NOT_FOUND",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":                       "models/" + modelName,
		"version":                    "001",
		"displayName":                modelName,
		"supportedGenerationMethods": []string{"generateContent", "countTokens", "embedContent"},
	})
}

// HandleGeminiAction handles POST /v1beta/models/*modelAction for Google Gemini SDKs.
func (h *Handler) HandleGeminiAction(c *gin.Context) {
	if !authenticateGemini(c) {
		return
	}

	param := strings.TrimPrefix(c.Param("modelAction"), "/")
	param = strings.TrimPrefix(param, "models/")

	parts := strings.Split(param, ":")
	modelName := parts[0]
	action := "generateContent"
	if len(parts) > 1 {
		action = parts[1]
	}

	isStream := strings.Contains(action, "streamGenerateContent") || c.Query("alt") == "sse"

	if !middleware.ValidateModelAllowed(c, modelName) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"code":    403,
				"message": fmt.Sprintf("API key not allowed to access model '%s'", modelName),
				"status":  "PERMISSION_DENIED",
			},
		})
		return
	}

	var geminiReq provider.GeminiRequest
	if err := c.ShouldBindJSON(&geminiReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": fmt.Sprintf("Failed to parse Gemini request: %v", err),
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	canonicalReq := convertInboundGeminiToCanonical(modelName, &geminiReq, isStream)

	sessionID := resolveSessionID(c, canonicalReq.Model, canonicalReq.Messages, "")
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()

	if !isStream {
		resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"code":    502,
					"message": err.Error(),
					"status":  "UNAVAILABLE",
				},
			})
			return
		}

		geminiResp := convertCanonicalToGeminiResponse(resp)
		dur := time.Since(start)
		pTokens := 0
		cTokens := 0
		if resp.Usage != nil {
			pTokens = resp.Usage.PromptTokens
			cTokens = resp.Usage.CompletionTokens
		}
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				SessionID:        sessionID,
				VirtualKey:       c.GetString(middleware.ContextKeyVirtualKey),
				TenantID:         c.GetString(middleware.ContextKeyTenant),
				Model:            modelName,
				PromptTokens:     pTokens,
				CompletionTokens: cTokens,
				TotalTokens:      pTokens + cTokens,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}
		c.JSON(http.StatusOK, geminiResp)
		return
	}

	streamChan, err := h.dispatcher.DispatchStream(reqCtx, canonicalReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"code":    502,
				"message": err.Error(),
				"status":  "UNAVAILABLE",
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

	w := c.Writer
	totalPromptTokens := 0
	totalCompTokens := 0
	firstTokenRecorded := false

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-streamChan:
			if !open {
				dur := time.Since(start)
				telemetry.GlobalMetrics.RecordRequest(true, dur, totalPromptTokens, totalCompTokens)
				if storage.GlobalAsyncLogger != nil {
					storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
						TraceID:          middleware.GetTraceID(c),
						SessionID:        sessionID,
						VirtualKey:       c.GetString(middleware.ContextKeyVirtualKey),
						TenantID:         c.GetString(middleware.ContextKeyTenant),
						Model:            modelName,
						PromptTokens:     totalPromptTokens,
						CompletionTokens: totalCompTokens,
						TotalTokens:      totalPromptTokens + totalCompTokens,
						DurationMs:       dur.Milliseconds(),
						StatusCode:       http.StatusOK,
					})
				}
				return
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

			if event.Chunk != nil && len(event.Chunk.Choices) > 0 {
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

func convertInboundGeminiToCanonical(modelName string, geminiReq *provider.GeminiRequest, stream bool) *model.ChatCompletionRequest {
	var msgs []model.ChatMessage

	if geminiReq.SystemInstruction != nil {
		var sysText strings.Builder
		for _, p := range geminiReq.SystemInstruction.Parts {
			sysText.WriteString(p.Text)
		}
		if sysText.Len() > 0 {
			msgs = append(msgs, model.ChatMessage{
				Role:    "system",
				Content: sysText.String(),
			})
		}
	}

	for _, content := range geminiReq.Contents {
		role := content.Role
		if role == "model" {
			role = "assistant"
		} else if role == "" {
			role = "user"
		}

		var textBuilder strings.Builder
		var toolCalls []model.ToolCall
		var hasFunctionResponse bool

		for _, p := range content.Parts {
			if p.Text != "" {
				textBuilder.WriteString(p.Text)
			}
			if p.FunctionCall != nil {
				argsBytes, _ := json.Marshal(p.FunctionCall.Args)
				toolCalls = append(toolCalls, model.ToolCall{
					ID:   fmt.Sprintf("call_%s_%d", p.FunctionCall.Name, time.Now().UnixNano()),
					Type: "function",
					Function: model.FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: string(argsBytes),
					},
				})
			}
			if p.FunctionResponse != nil {
				hasFunctionResponse = true
				respBytes, _ := json.Marshal(p.FunctionResponse.Response)
				msgs = append(msgs, model.ChatMessage{
					Role:       "tool",
					Name:       p.FunctionResponse.Name,
					ToolCallID: p.FunctionResponse.Name,
					Content:    string(respBytes),
				})
			}
		}

		if !hasFunctionResponse {
			msgs = append(msgs, model.ChatMessage{
				Role:      role,
				Content:   textBuilder.String(),
				ToolCalls: toolCalls,
			})
		}
	}

	var tools []model.Tool
	for _, tc := range geminiReq.Tools {
		for _, fd := range tc.FunctionDeclarations {
			params := fd.Parameters
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, model.Tool{
				Type: "function",
				Function: map[string]any{
					"name":        fd.Name,
					"description": fd.Description,
					"parameters":  params,
				},
			})
		}
	}

	req := &model.ChatCompletionRequest{
		Model:    modelName,
		Messages: msgs,
		Stream:   stream,
		Tools:    tools,
	}

	if geminiReq.GenerationConfig != nil {
		req.Temperature = geminiReq.GenerationConfig.Temperature
		req.TopP = geminiReq.GenerationConfig.TopP
		req.MaxTokens = geminiReq.GenerationConfig.MaxOutputTokens
	}

	return req
}

func convertCanonicalToGeminiResponse(resp *model.ChatCompletionResponse) gin.H {
	replyText := ""
	var toolCalls []model.ToolCall
	finishReason := "STOP"

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		replyText = choice.Message.GetContentString()
		toolCalls = choice.Message.ToolCalls
		if choice.FinishReason != nil && *choice.FinishReason == "length" {
			finishReason = "MAX_TOKENS"
		}
	}

	var parts []gin.H
	if replyText != "" {
		parts = append(parts, gin.H{"text": replyText})
	}
	for _, tc := range toolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		parts = append(parts, gin.H{
			"functionCall": gin.H{
				"name": tc.Function.Name,
				"args": args,
			},
		})
	}

	promptTokens := 0
	compTokens := 0
	totalTokens := 0
	if resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		compTokens = resp.Usage.CompletionTokens
		totalTokens = resp.Usage.TotalTokens
	}

	return gin.H{
		"candidates": []gin.H{
			{
				"content": gin.H{
					"role":  "model",
					"parts": parts,
				},
				"finishReason": finishReason,
				"index":        0,
			},
		},
		"usageMetadata": gin.H{
			"promptTokenCount":     promptTokens,
			"candidatesTokenCount": compTokens,
			"totalTokenCount":      totalTokens,
		},
	}
}
