package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// HandleResponses handles OpenAI Responses API POST /v1/responses.
func (h *Handler) HandleResponses(c *gin.Context) {
	var req model.ResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if respondRequestTooLarge(c, err) {
			return
		}
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

	if !validateModelCurrency(c, req.Model) {
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

	sessionID := sanitizeSessionID(req.PreviousResponseID)
	if sessionID == "" {
		sessionID = resolveSessionID(c, chatReq.Model, chatReq.Messages, req.User)
	} else {
		sessionID = "resp_" + sessionID
		publishSession(c, sessionID)
	}
	// The provider call is detached from the downstream request so a customer
	// disconnect cannot cancel it before its usage report arrives.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionAffinityKey(c, sessionID))
	}

	start := time.Now()

	// Non-streaming execution
	if !req.Stream {
		h.handleResponsesNonStreaming(c, reqCtx, req, chatReq, sessionID, start)
		return
	}

	// Streaming SSE execution
	h.handleResponsesStreaming(c, reqCtx, req, chatReq, sessionID, start)
}

func (h *Handler) handleResponsesNonStreaming(
	c *gin.Context,
	reqCtx context.Context,
	req model.ResponseRequest,
	chatReq *model.ChatCompletionRequest,
	sessionID string,
	start time.Time,
) {
	resp, err := h.dispatcher.Dispatch(reqCtx, chatReq)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}

	dur := time.Since(start)
	pTokens, cTokens, cachedTokens, usageSource := resolveChatUsage(chatReq, resp)

	RecordUsage(c, AuditRecordParams{
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		CachedTokens:     cachedTokens,
		Duration:         dur,
		StatusCode:       http.StatusOK,
		UsageSource:      usageSource,
	})

	result := model.ConvertChatResponseToResponseResult(resp)
	c.JSON(http.StatusOK, result)
}
