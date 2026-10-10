package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
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

	// Protocol format and content validation per-model rules and optional per-key overrides
	valRes := validator.ValidateOpenAIRequest(&req, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		valRes.Error.WriteGinResponse(c)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
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

	if !req.Stream {
		h.handleChatUnary(c, reqCtx, &req, sessionID, start)
		return
	}

	h.handleChatStream(c, reqCtx, &req, sessionID, start)
}

func (h *Handler) handleChatUnary(c *gin.Context, reqCtx context.Context, req *model.ChatCompletionRequest, sessionID string, start time.Time) {
	resp, err := h.dispatcher.Dispatch(reqCtx, req)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}
	dur := time.Since(start)
	// Audit chat_id is the raw upstream response ID only — a fabricated
	// fallback is written into resp.ID for OpenAI protocol compliance but
	// never persisted (it would never reconcile with the provider).
	upstreamChatID := resp.ID
	if resp.ID == "" {
		resp.ID = fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())
	}
	if req.Model != "" {
		resp.Model = req.Model
	}
	pTokens, cTokens, cachedTokens := 0, 0, 0
	if resp.Usage != nil {
		pTokens = resp.Usage.PromptTokens
		cTokens = resp.Usage.CompletionTokens
		cachedTokens = resp.Usage.GetCachedTokens()
	}
	RecordUsage(c, AuditRecordParams{
		ChatID:           upstreamChatID,
		Channel:          resp.Channel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		CachedTokens:     cachedTokens,
		Duration:         dur,
		StatusCode:       http.StatusOK,
	})
	c.JSON(http.StatusOK, resp)
}
