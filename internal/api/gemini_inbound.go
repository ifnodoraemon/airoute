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
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

// HandleGeminiModels handles GET /v1beta/models.
func (h *Handler) HandleGeminiModels(c *gin.Context) {
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

	// Validate Gemini protocol and schema rules with optional key config
	valRes := validator.ValidateGeminiRequest(modelName, &geminiReq, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		valRes.Error.WriteGinResponse(c)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
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
		var cost float64
		var isOffPeak bool
		var offPeakDiscount float64 = 1.0
		keyGroup := getKeyGroup(c)
		if billing.GlobalEngine != nil {
			cost, _, _, isOffPeak, offPeakDiscount = billing.GlobalEngine.CalculateCostDetailedWithGroup(modelName, keyGroup, pTokens, cTokens, 0, time.Now())
		}
		telemetry.GlobalMetrics.RecordRequestWithModel(modelName, true, dur, pTokens, cTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				SessionID:        sessionID,
				APIKey:           getRequestAPIKey(c),
				TenantID:         c.GetString(middleware.ContextKeyTenant),
				Model:            modelName,
				PromptTokens:     pTokens,
				CompletionTokens: cTokens,
				TotalTokens:      pTokens + cTokens,
				Cost:             cost,
				IsOffPeak:        isOffPeak,
				OffPeakDiscount:  offPeakDiscount,
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

	var recordOnce sync.Once
	recordStreamEnd := func() {
		dur := time.Since(start)
		var cost float64
		var isOffPeak bool
		var offPeakDiscount float64 = 1.0
		keyGroup := getKeyGroup(c)
		if billing.GlobalEngine != nil {
			cost, _, _, isOffPeak, offPeakDiscount = billing.GlobalEngine.CalculateCostDetailedWithGroup(modelName, keyGroup, totalPromptTokens, totalCompTokens, 0, time.Now())
		}
		telemetry.GlobalMetrics.RecordRequest(true, dur, totalPromptTokens, totalCompTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				SessionID:        sessionID,
				APIKey:           getRequestAPIKey(c),
				TenantID:         c.GetString(middleware.ContextKeyTenant),
				Model:            modelName,
				PromptTokens:     totalPromptTokens,
				CompletionTokens: totalCompTokens,
				TotalTokens:      totalPromptTokens + totalCompTokens,
				Cost:             cost,
				IsOffPeak:        isOffPeak,
				OffPeakDiscount:  offPeakDiscount,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}
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

var defaultGeminiAdapter = adapter.NewGeminiAdapter()

func convertInboundGeminiToCanonical(modelName string, geminiReq *provider.GeminiRequest, stream bool) *model.ChatCompletionRequest {
	req, err := defaultGeminiAdapter.ToCanonical(context.Background(), geminiReq)
	if err != nil {
		return &model.ChatCompletionRequest{Model: modelName, Stream: stream}
	}
	req.Model = modelName
	req.Stream = stream
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
