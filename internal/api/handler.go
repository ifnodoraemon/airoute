package api

import (
	"context"
	"errors"
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

// isRequestBodyTooLarge reports whether a bind failure came from the request
// body exceeding middleware.BodyLimit's cap (http.MaxBytesReader) rather than
// from malformed JSON. The two are different client errors — 413 vs 400 — and
// collapsing them into "Invalid JSON request body" told clients to fix their
// JSON when the payload was merely too big.
func isRequestBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// respondRequestTooLarge writes the OpenAI-shaped 413 response when the bind
// error was a body-limit violation, reporting whether it handled the response.
func respondRequestTooLarge(c *gin.Context, err error) bool {
	if !isRequestBodyTooLarge(err) {
		return false
	}
	RespondOpenAIError(c, http.StatusRequestEntityTooLarge,
		"Request body is too large for this endpoint. Reduce it or configure GATEWAY_MAX_BODY_BYTES.",
		"invalid_request_error", "payload_too_large")
	return true
}

// HandleChatCompletions handles POST /v1/chat/completions.
func (h *Handler) HandleChatCompletions(c *gin.Context) {
	CaptureRequestSnapshot(c)
	var req model.ChatCompletionRequest
	native, err := bindNativeJSON(c, &req, model.NativeChat)
	if err != nil {
		if respondRequestTooLarge(c, err) {
			return
		}
		RespondOpenAIError(c, http.StatusBadRequest, fmt.Sprintf("Invalid JSON request body: %v", err), "invalid_request_error", "invalid_payload")
		return
	}
	req.Native = native

	if req.Model == "" {
		RespondOpenAIError(c, http.StatusBadRequest, "Missing 'model' field in request body", "invalid_request_error", "missing_model")
		return
	}

	// Validate authorization for requested model
	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model), "forbidden", "model_not_allowed")
		return
	}

	// Protocol format and content validation per-model rules and optional per-key overrides
	valRes := validator.ValidateOpenAIRequest(&req, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		rejectInvalidRequest(c, valRes.Error)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	// Extract or derive zero-touch LLM Session Affinity key
	sessionID := resolveSessionID(c, req.Model, req.Messages, req.User)
	// The provider call is detached from the downstream request so a customer
	// disconnect cannot cancel it before its usage report arrives.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()
	if sessionID != "" {
		// A18: the routing key is namespaced by the authenticated caller so a
		// client-supplied session identifier can never steer another tenant's
		// channel affinity.
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionAffinityKey(c, sessionID))
	}

	start := time.Now()

	if !req.Stream {
		h.handleChatUnary(c, reqCtx, &req, sessionID, start)
		return
	}

	h.handleChatStream(c, reqCtx, &req, sessionID, start)
}

func (h *Handler) handleChatUnary(c *gin.Context, reqCtx context.Context, req *model.ChatCompletionRequest, sessionID string, start time.Time) {
	if !topUpQuotaForRequest(c, req) {
		return
	}
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
		resp.ID = fmt.Sprintf("chatcmpl-%x", time.Now().UnixMicro())
	}
	if req.Model != "" {
		resp.Model = req.Model
	}
	// A12: publish the upstream chat id on the gin context so the trace archiver
	// can persist it (it previously read a key nothing ever wrote).
	if upstreamChatID != "" {
		c.Set(ContextKeyChatID, upstreamChatID)
	}
	pTokens, cTokens, cachedTokens, usageSource := resolveChatUsage(req, resp)
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
		UsageSource:      usageSource,
	})
	RecordSuccessTrace(c, req.Model, resp.Channel, dur, resp)
	if len(resp.NativeResponse) > 0 {
		c.Data(http.StatusOK, "application/json", resp.NativeResponse)
		return
	}
	c.JSON(http.StatusOK, resp)
}
