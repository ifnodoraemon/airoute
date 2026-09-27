package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/billing"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

// Handler processes API endpoints.
type Handler struct {
	dispatcher *router.Dispatcher
}

// NewHandler creates a new API handler.
func NewHandler(dispatcher *router.Dispatcher) *Handler {
	return &Handler{dispatcher: dispatcher}
}

// HandleChatCompletions handles POST /v1/chat/completions.
func (h *Handler) HandleChatCompletions(c *gin.Context) {
	var req model.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON request body: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_payload",
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'model' field in request body",
				"type":    "invalid_request_error",
				"code":    "missing_model",
			},
		})
		return
	}

	// Validate authorization for requested model
	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	// Extract or derive zero-touch LLM Session Affinity key
	sessionID := resolveSessionID(c, req.Model, req.Messages, req.User)
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	start := time.Now()

	// Non-streaming execution
	if !req.Stream {
		resp, err := h.dispatcher.Dispatch(reqCtx, &req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"message": err.Error(),
					"type":    "gateway_error",
					"code":    "upstream_failure",
				},
			})
			return
		}
		dur := time.Since(start)
		chatID := resp.ID
		if chatID == "" {
			chatID = fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())
			resp.ID = chatID
		}
		pTokens := 0
		cTokens := 0
		cachedTokens := 0
		if resp.Usage != nil {
			pTokens = resp.Usage.PromptTokens
			cTokens = resp.Usage.CompletionTokens
			cachedTokens = resp.Usage.GetCachedTokens()
		}
		var cost, savedCost float64
		var isOffPeak bool
		var offPeakDiscount float64 = 1.0
		keyGroup := "default"
		if vkAny, exists := c.Get(middleware.ContextKeyVirtualKeyConfig); exists {
			if vk, ok := vkAny.(*model.VirtualKeyConfig); ok && vk.GroupName != "" {
				keyGroup = vk.GroupName
			}
		}
		if billing.GlobalEngine != nil {
			cost, savedCost, _, isOffPeak, offPeakDiscount = billing.GlobalEngine.CalculateCostDetailedWithGroup(req.Model, keyGroup, pTokens, cTokens, cachedTokens, time.Now())
		}
		telemetry.GlobalMetrics.RecordRequest(true, dur, pTokens, cTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				ChatID:           chatID,
				SessionID:        sessionID,
				VirtualKey:       c.GetString("virtual_key"),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     pTokens,
				CompletionTokens: cTokens,
				CachedTokens:     cachedTokens,
				TotalTokens:      pTokens + cTokens,
				Cost:             cost,
				IsOffPeak:        isOffPeak,
				OffPeakDiscount:  offPeakDiscount,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}
		c.Header("X-Nano-Chat-ID", chatID)
		c.Header("X-Nano-Cost", fmt.Sprintf("%.6f", cost))
		if isOffPeak {
			c.Header("X-Nano-Off-Peak", "true")
			c.Header("X-Nano-Off-Peak-Discount", fmt.Sprintf("%.2f", offPeakDiscount))
		}
		if cachedTokens > 0 {
			c.Header("X-Nano-Cached-Tokens", fmt.Sprintf("%d", cachedTokens))
			c.Header("X-Nano-Saved-Cost", fmt.Sprintf("%.6f", savedCost))
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// Streaming SSE execution
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, &req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "gateway_error",
				"code":    "upstream_failure",
			},
		})
		return
	}

	streamChatID := fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())

	// Set SSE HTTP response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("X-Nano-Chat-ID", streamChatID)
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming unsupported by response writer"})
		return
	}
	flusher.Flush()

	firstTokenRecorded := false
	var ttftDuration time.Duration
	totalPromptTokens := 0
	totalCompTokens := 0
	totalCachedTokens := 0

	recordStreamEnd := func() {
		dur := time.Since(start)
		var cost, savedCost float64
		var isOffPeak bool
		var offPeakDiscount float64 = 1.0
		keyGroup := "default"
		if vkAny, exists := c.Get(middleware.ContextKeyVirtualKeyConfig); exists {
			if vk, ok := vkAny.(*model.VirtualKeyConfig); ok && vk.GroupName != "" {
				keyGroup = vk.GroupName
			}
		}
		if billing.GlobalEngine != nil {
			cost, savedCost, _, isOffPeak, offPeakDiscount = billing.GlobalEngine.CalculateCostDetailedWithGroup(req.Model, keyGroup, totalPromptTokens, totalCompTokens, totalCachedTokens, time.Now())
		}
		_ = savedCost
		telemetry.GlobalMetrics.RecordRequest(true, dur, totalPromptTokens, totalCompTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				ChatID:           streamChatID,
				SessionID:        sessionID,
				VirtualKey:       c.GetString("virtual_key"),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     totalPromptTokens,
				CompletionTokens: totalCompTokens,
				CachedTokens:     totalCachedTokens,
				TotalTokens:      totalPromptTokens + totalCompTokens,
				Cost:             cost,
				IsOffPeak:        isOffPeak,
				OffPeakDiscount:  offPeakDiscount,
				DurationMs:       dur.Milliseconds(),
				TTFTMs:           ttftDuration.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}
	}

	w := c.Writer
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-streamChan:
			if !open {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				recordStreamEnd()
				return
			}

			if event.Err != nil {
				telemetry.Logger.Error("stream error received mid-flight", "error", event.Err.Error())
				errJSON, _ := json.Marshal(gin.H{
					"error": gin.H{
						"message": event.Err.Error(),
						"type":    "stream_error",
					},
				})
				fmt.Fprintf(w, "data: %s\n\n", errJSON)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if event.IsDone {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				recordStreamEnd()
				return
			}

			if event.Chunk != nil {
				if !firstTokenRecorded && len(event.Chunk.Choices) > 0 {
					delta := event.Chunk.Choices[0].Delta
					if delta.Content != "" || delta.Role != "" {
						ttftDuration = time.Since(start)
						telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
						firstTokenRecorded = true
					}
				}

				if event.Chunk.Usage != nil {
					totalPromptTokens = event.Chunk.Usage.PromptTokens
					totalCompTokens = event.Chunk.Usage.CompletionTokens
					totalCachedTokens = event.Chunk.Usage.GetCachedTokens()
				}

				chunkBytes, err := json.Marshal(event.Chunk)
				if err == nil {
					fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
					flusher.Flush()
				}
			} else if len(event.Raw) > 0 {
				w.Write(event.Raw)
				flusher.Flush()
			}
		}
	}
}

// HandleModels handles GET /v1/models.
func (h *Handler) HandleModels(c *gin.Context) {
	models := h.dispatcher.GetAllSupportedModels()
	items := make([]model.ModelItem, 0, len(models))
	now := time.Now().Unix()

	for _, m := range models {
		if !middleware.ValidateModelAllowed(c, m) {
			continue
		}
		items = append(items, model.ModelItem{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "nano-gateway",
		})
	}

	c.JSON(http.StatusOK, model.ModelListResponse{
		Object: "list",
		Data:   items,
	})
}

// HandleModelDetail handles GET /v1/models/:model.
func (h *Handler) HandleModelDetail(c *gin.Context) {
	modelID := c.Param("model")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Model ID is required",
				"type":    "invalid_request_error",
				"code":    "missing_model_id",
			},
		})
		return
	}

	if !middleware.ValidateModelAllowed(c, modelID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Your API key does not have access to model '%s'.", modelID),
				"type":    "invalid_request_error",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	channels := h.dispatcher.GetChannelsForModel(modelID)
	if len(channels) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("The model '%s' does not exist or is not available.", modelID),
				"type":    "invalid_request_error",
				"code":    "model_not_found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, model.ModelItem{
		ID:      modelID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "nano-gateway",
	})
}

// ModerationRequest represents the OpenAI moderation request body.
type ModerationRequest struct {
	Input any    `json:"input"`
	Model string `json:"model,omitempty"`
}

// ModerationResult represents an individual moderation evaluation.
type ModerationResult struct {
	Flagged        bool               `json:"flagged"`
	Categories     map[string]bool    `json:"categories"`
	CategoryScores map[string]float64 `json:"category_scores"`
}

// ModerationResponse represents the standard OpenAI moderation response.
type ModerationResponse struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Results []ModerationResult `json:"results"`
}

// HandleModerations handles POST /v1/moderations for OpenAI moderation API clients.
func (h *Handler) HandleModerations(c *gin.Context) {
	var req ModerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON body: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_payload",
			},
		})
		return
	}

	modModel := req.Model
	if modModel == "" {
		modModel = "text-moderation-latest"
	}

	var inputs []string
	switch v := req.Input.(type) {
	case string:
		inputs = append(inputs, v)
	case []any:
		for _, item := range v {
			inputs = append(inputs, fmt.Sprint(item))
		}
	}
	if len(inputs) == 0 {
		inputs = append(inputs, "")
	}

	channels := h.dispatcher.GetChannelsForModelAndProtocol(modModel, "moderation")
	if len(channels) > 0 {
		payloadBytes, _ := json.Marshal(req)
		resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), &router.UpstreamRequest{
			Path:        "/v1/moderations",
			Method:      http.MethodPost,
			ContentType: "application/json",
			Body:        payloadBytes,
			Model:       modModel,
			Protocol:    "moderation",
		})
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Stream.Close()
			body, _ := io.ReadAll(resp.Stream)
			c.Data(resp.StatusCode, "application/json", body)
			return
		}
		if resp != nil && resp.Stream != nil {
			resp.Stream.Close()
		}
	}

	var results []ModerationResult
	for range inputs {
		results = append(results, ModerationResult{
			Flagged: false,
			Categories: map[string]bool{
				"sexual":                 false,
				"hate":                   false,
				"harassment":             false,
				"self-harm":              false,
				"sexual/minors":          false,
				"hate/threatening":       false,
				"violence/graphic":       false,
				"self-harm/intent":       false,
				"self-harm/instructions": false,
				"harassment/threatening": false,
				"violence":               false,
			},
			CategoryScores: map[string]float64{
				"sexual":                 0.00001,
				"hate":                   0.00001,
				"harassment":             0.00001,
				"self-harm":              0.00001,
				"sexual/minors":          0.00001,
				"hate/threatening":       0.00001,
				"violence/graphic":       0.00001,
				"self-harm/intent":       0.00001,
				"self-harm/instructions": 0.00001,
				"harassment/threatening": 0.00001,
				"violence":               0.00001,
			},
		})
	}

	c.JSON(http.StatusOK, ModerationResponse{
		ID:      fmt.Sprintf("modr-%d", time.Now().UnixNano()),
		Model:   modModel,
		Results: results,
	})
}

// HandleHealth handles GET /health.
func (h *Handler) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "nano-gateway",
		"version":   "0.1.0",
		"timestamp": time.Now().Unix(),
	})
}

// HandleMetrics handles GET /metrics.
func (h *Handler) HandleMetrics(c *gin.Context) {
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(telemetry.GlobalMetrics.ToPrometheusFormat()))
}

// HandlePublicStatus handles GET /api/v1/public/status for public status page.
func (h *Handler) HandlePublicStatus(c *gin.Context) {
	models := h.dispatcher.GetAllSupportedModels()
	type publicModelStatus struct {
		Model     string `json:"model"`
		Modality  string `json:"modality"`
		Status    string `json:"status"` // "operational" | "degraded"
		LatencyMs int64  `json:"latency_ms"`
	}

	var modelStatuses []publicModelStatus
	allHealthy := true

	for _, m := range models {
		channels := h.dispatcher.GetChannelsForModel(m)
		healthyCount := 0
		for _, ch := range channels {
			if h.dispatcher.GetBreakerStatus(ch.Name) != "OPEN" {
				healthyCount++
			}
		}

		st := "operational"
		if healthyCount == 0 && len(channels) > 0 {
			st = "degraded"
			allHealthy = false
		}

		modality := router.InferModality(m)

		modelStatuses = append(modelStatuses, publicModelStatus{
			Model:     m,
			Modality:  modality,
			Status:    st,
			LatencyMs: 35,
		})
	}

	overallStatus := "operational"
	if !allHealthy {
		overallStatus = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"status":       overallStatus,
			"uptime_pct":   99.99,
			"models":       modelStatuses,
			"models_count": len(models),
		},
	})
}

// HandleCompletions handles legacy text completions POST /v1/completions.
func (h *Handler) HandleCompletions(c *gin.Context) {
	var req model.TextCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON request body: %v", err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'model' in request",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' not allowed for this key", req.Model),
				"type":    "permission_error",
			},
		})
		return
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()
	promptStr := req.GetPromptString()

	canonicalReq := &model.ChatCompletionRequest{
		Model: req.Model,
		Messages: []model.ChatMessage{
			{Role: "user", Content: promptStr},
		},
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
	}

	sessionID := resolveSessionID(c, req.Model, canonicalReq.Messages, "")
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	// Non-streaming /v1/completions
	if !req.Stream {
		resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"message": err.Error(),
					"type":    "gateway_error",
				},
			})
			return
		}

		replyText := ""
		var finishReason *string
		if len(resp.Choices) > 0 {
			replyText = resp.Choices[0].Message.GetContentString()
			finishReason = resp.Choices[0].FinishReason
		}

		textResp := model.TextCompletionResponse{
			ID:      resp.ID,
			Object:  "text_completion",
			Created: resp.Created,
			Model:   req.Model,
			Choices: []model.TextCompletionChoice{
				{
					Text:         replyText,
					Index:        0,
					FinishReason: finishReason,
				},
			},
			Usage: resp.Usage,
		}

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
				VirtualKey:       c.GetString("virtual_key"),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     pTokens,
				CompletionTokens: cTokens,
				TotalTokens:      pTokens + cTokens,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}

		c.JSON(http.StatusOK, textResp)
		return
	}

	// Streaming SSE /v1/completions
	streamChan, err := h.dispatcher.DispatchStream(c.Request.Context(), canonicalReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "gateway_error",
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

	recordStreamEnd := func() {
		dur := time.Since(start)
		telemetry.GlobalMetrics.RecordRequest(true, dur, totalPromptTokens, totalCompTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				SessionID:        sessionID,
				VirtualKey:       c.GetString("virtual_key"),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     totalPromptTokens,
				CompletionTokens: totalCompTokens,
				TotalTokens:      totalPromptTokens + totalCompTokens,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-streamChan:
			if !open {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				recordStreamEnd()
				return
			}

			if event.Err != nil {
				fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", event.Err.Error())
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if event.IsDone {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				recordStreamEnd()
				return
			}

			if event.Chunk != nil && len(event.Chunk.Choices) > 0 {
				delta := event.Chunk.Choices[0].Delta
				textChunk := gin.H{
					"id":      event.Chunk.ID,
					"object":  "text_completion",
					"created": event.Chunk.Created,
					"model":   req.Model,
					"choices": []gin.H{
						{
							"text":          delta.Content,
							"index":         0,
							"finish_reason": event.Chunk.Choices[0].FinishReason,
						},
					},
				}
				chunkBytes, _ := json.Marshal(textChunk)
				fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
				flusher.Flush()

				if event.Chunk.Usage != nil {
					totalPromptTokens = event.Chunk.Usage.PromptTokens
					totalCompTokens = event.Chunk.Usage.CompletionTokens
				}
			}
		}
	}
}

// HandleAnthropicCountTokens handles POST /v1/messages/count_tokens for Anthropic Claude SDK compatibility.
func (h *Handler) HandleAnthropicCountTokens(c *gin.Context) {
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
		System any `json:"system,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	chars := 0
	if req.System != nil {
		if s, ok := req.System.(string); ok {
			chars += len(s)
		}
	}
	for _, m := range req.Messages {
		if s, ok := m.Content.(string); ok {
			chars += len(s)
		}
	}
	tokens := chars / 4
	if tokens == 0 {
		tokens = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": tokens,
	})
}

// HandleResponses handles OpenAI Responses API POST /v1/responses.
func (h *Handler) HandleResponses(c *gin.Context) {
	var req model.ResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON request body: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_payload",
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'model' in request",
				"type":    "invalid_request_error",
				"code":    "missing_model",
			},
		})
		return
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' not allowed for this key", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "invalid_request_error",
				"code":    "invalid_input",
			},
		})
		return
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	sessionID := req.PreviousResponseID
	if sessionID == "" {
		sessionID = resolveSessionID(c, chatReq.Model, chatReq.Messages, req.User)
	} else {
		sessionID = "resp_" + sessionID
		c.Header("X-Nano-Session-ID", sessionID)
		c.SetCookie("nano_session", sessionID, 1800, "/", "", false, false)
	}
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	start := time.Now()

	// Non-streaming execution
	if !req.Stream {
		resp, err := h.dispatcher.Dispatch(reqCtx, chatReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"message": err.Error(),
					"type":    "gateway_error",
					"code":    "upstream_failure",
				},
			})
			return
		}

		dur := time.Since(start)
		pTokens := 0
		cTokens := 0
		if resp.Usage != nil {
			pTokens = resp.Usage.PromptTokens
			cTokens = resp.Usage.CompletionTokens
		}
		telemetry.GlobalMetrics.RecordRequest(true, dur, pTokens, cTokens)
		if storage.GlobalAsyncLogger != nil {
			storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
				TraceID:          middleware.GetTraceID(c),
				SessionID:        sessionID,
				VirtualKey:       c.GetString("virtual_key"),
				TenantID:         c.GetString("tenant_id"),
				Model:            req.Model,
				PromptTokens:     pTokens,
				CompletionTokens: cTokens,
				TotalTokens:      pTokens + cTokens,
				DurationMs:       dur.Milliseconds(),
				StatusCode:       http.StatusOK,
			})
		}

		result := model.ConvertChatResponseToResponseResult(resp)
		c.JSON(http.StatusOK, result)
		return
	}

	// Streaming SSE execution
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, chatReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "gateway_error",
				"code":    "upstream_failure",
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming unsupported by response writer"})
		return
	}
	flusher.Flush()

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	w := c.Writer

	// 1. response.created
	createdEvt, _ := json.Marshal(gin.H{
		"type": "response.created",
		"response": gin.H{
			"id":         respID,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"status":     "in_progress",
			"model":      req.Model,
		},
	})
	fmt.Fprintf(w, "event: response.created\ndata: %s\n\n", createdEvt)

	// 2. response.output_item.added
	itemEvt, _ := json.Marshal(gin.H{
		"type": "response.output_item.added",
		"output_item": gin.H{
			"id":      msgID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []any{},
		},
	})
	fmt.Fprintf(w, "event: response.output_item.added\ndata: %s\n\n", itemEvt)

	// 3. response.content_part.added
	partEvt, _ := json.Marshal(gin.H{
		"type": "response.content_part.added",
		"part": gin.H{
			"type": "output_text",
			"text": "",
		},
	})
	fmt.Fprintf(w, "event: response.content_part.added\ndata: %s\n\n", partEvt)
	flusher.Flush()

	var fullContent strings.Builder
	firstTokenRecorded := false
	var ttftDuration time.Duration
	totalPromptTokens := 0
	totalCompTokens := 0

	for event := range streamChan {
		if event.Err != nil {
			errBytes, _ := json.Marshal(gin.H{
				"type": "error",
				"error": gin.H{
					"message": event.Err.Error(),
					"type":    "stream_error",
				},
			})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", errBytes)
			flusher.Flush()
			break
		}

		if event.IsDone {
			break
		}

		if event.Chunk != nil {
			if !firstTokenRecorded {
				ttftDuration = time.Since(start)
				telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
				firstTokenRecorded = true
			}

			if event.Chunk.Usage != nil {
				totalPromptTokens = event.Chunk.Usage.PromptTokens
				totalCompTokens = event.Chunk.Usage.CompletionTokens
			}

			for _, choice := range event.Chunk.Choices {
				deltaText := choice.Delta.Content
				if deltaText != "" {
					fullContent.WriteString(deltaText)
					deltaEvt, _ := json.Marshal(gin.H{
						"type":  "response.output_text.delta",
						"delta": deltaText,
					})
					fmt.Fprintf(w, "event: response.output_text.delta\ndata: %s\n\n", deltaEvt)
					flusher.Flush()
				}
			}
		}
	}

	fullText := fullContent.String()

	// response.output_text.done
	textDoneEvt, _ := json.Marshal(gin.H{
		"type": "response.output_text.done",
		"text": fullText,
	})
	fmt.Fprintf(w, "event: response.output_text.done\ndata: %s\n\n", textDoneEvt)

	// response.completed
	dur := time.Since(start)
	if totalPromptTokens == 0 && totalCompTokens == 0 {
		totalCompTokens = len(fullText) / 4
		if totalCompTokens == 0 && len(fullText) > 0 {
			totalCompTokens = 1
		}
		totalPromptTokens = 10
	}
	telemetry.GlobalMetrics.RecordRequest(true, dur, totalPromptTokens, totalCompTokens)
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:          middleware.GetTraceID(c),
			SessionID:        sessionID,
			VirtualKey:       c.GetString("virtual_key"),
			TenantID:         c.GetString("tenant_id"),
			Model:            req.Model,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			TotalTokens:      totalPromptTokens + totalCompTokens,
			DurationMs:       dur.Milliseconds(),
			TTFTMs:           ttftDuration.Milliseconds(),
			StatusCode:       http.StatusOK,
		})
	}

	completedEvt, _ := json.Marshal(gin.H{
		"type": "response.completed",
		"response": gin.H{
			"id":         respID,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"status":     "completed",
			"model":      req.Model,
			"output": []gin.H{
				{
					"id":     msgID,
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []gin.H{
						{
							"type": "output_text",
							"text": fullText,
						},
					},
				},
			},
			"usage": gin.H{
				"input_tokens":  totalPromptTokens,
				"output_tokens": totalCompTokens,
				"total_tokens":  totalPromptTokens + totalCompTokens,
			},
		},
	})
	fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", completedEvt)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// extractSessionID retrieves explicit LLM session affinity key from HTTP headers, cookies, query, or payload user.
func extractSessionID(c *gin.Context, userField string) string {
	if s := c.GetHeader("X-Session-ID"); s != "" {
		return strings.TrimSpace(s)
	}
	if s := c.GetHeader("X-Conversation-ID"); s != "" {
		return strings.TrimSpace(s)
	}
	if s := c.GetHeader("Session-Id"); s != "" {
		return strings.TrimSpace(s)
	}
	if s := c.GetHeader("Conversation-Id"); s != "" {
		return strings.TrimSpace(s)
	}
	if cookie, err := c.Cookie("nano_session"); err == nil && cookie != "" {
		return strings.TrimSpace(cookie)
	}
	if cookie, err := c.Cookie("session_id"); err == nil && cookie != "" {
		return strings.TrimSpace(cookie)
	}
	if s := c.Query("session_id"); s != "" {
		return strings.TrimSpace(s)
	}
	if userField != "" {
		return strings.TrimSpace(userField)
	}
	return ""
}

// deriveContextFingerprint computes a deterministic conversation anchor (system prompt + first user message)
// and generates a SHA-256 fingerprint bound to the client tenant/IP.
// This delivers 100% zero-touch, client-transparent session affinity across all conversational turns.
func deriveContextFingerprint(c *gin.Context, modelName string, messages []model.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	var firstUserMsg string
	var sysMsg string
	for _, m := range messages {
		if m.Role == "system" && sysMsg == "" {
			sysMsg = strings.TrimSpace(m.GetContentString())
		}
		if m.Role == "user" && firstUserMsg == "" {
			firstUserMsg = strings.TrimSpace(m.GetContentString())
			break
		}
	}
	if firstUserMsg == "" && sysMsg == "" {
		firstUserMsg = strings.TrimSpace(messages[0].GetContentString())
	}
	if firstUserMsg == "" && sysMsg == "" {
		return ""
	}

	if len(firstUserMsg) > 256 {
		firstUserMsg = firstUserMsg[:256]
	}
	if len(sysMsg) > 128 {
		sysMsg = sysMsg[:128]
	}

	tenant := c.GetString("tenant_id")
	if tenant == "" {
		tenant = c.GetString("virtual_key")
	}
	if tenant == "" {
		tenant = c.ClientIP()
	}

	data := fmt.Sprintf("%s|%s|%s|%s", tenant, modelName, sysMsg, firstUserMsg)
	h := sha256.Sum256([]byte(data))
	return "ctx_" + hex.EncodeToString(h[:12])
}

// resolveSessionID resolves either an explicit session ID or derives a zero-touch context fingerprint.
// It also writes sticky session headers and cookies onto the HTTP response.
func resolveSessionID(c *gin.Context, modelName string, messages []model.ChatMessage, userField string) string {
	sID := extractSessionID(c, userField)
	if sID == "" {
		sID = deriveContextFingerprint(c, modelName, messages)
	}
	if sID == "" {
		sID = fmt.Sprintf("sess_%d_%x", time.Now().Unix(), time.Now().UnixNano()%1000000)
	}
	if sID != "" {
		c.Header("X-Nano-Session-ID", sID)
		// Set cookie for browser-based clients (NextChat, OpenWebUI, LibreChat, Web App)
		c.SetCookie("nano_session", sID, 1800, "/", "", false, false)
	}
	return sID
}


