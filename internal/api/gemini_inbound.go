package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/adapter"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

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

	if !validateModelCurrency(c, modelName) {
		return
	}
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

	// A08: :embedContent is a real embeddings call, not a chat completion. It
	// must be parsed and dispatched through the embeddings path; previously the
	// canonical chat converter silently turned it into a generateContent request
	// and the client received chat candidates.
	if action == "batchGenerateContent" || action == "asyncBatchEmbedContent" || action == "batchEmbedContents" {
		h.handleGeminiBatchAction(c, modelName, action)
		return
	}
	if action == "embedContent" {
		h.handleGeminiEmbedContent(c, modelName)
		return
	}
	if action != "generateContent" && action != "streamGenerateContent" && action != "countTokens" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": fmt.Sprintf("unsupported Gemini method '%s' for model '%s'", action, modelName),
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	var geminiReq provider.GeminiRequest
	native, err := bindNativeJSON(c, &geminiReq, model.NativeGemini)
	if err != nil {
		if isRequestBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": gin.H{
					"code":    http.StatusRequestEntityTooLarge,
					"message": "Request body is too large for this endpoint. Reduce it or configure GATEWAY_MAX_BODY_BYTES.",
					"status":  "INVALID_ARGUMENT",
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": fmt.Sprintf("Failed to parse Gemini request: %v", err),
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	if action == "countTokens" {
		h.handleGeminiCount(c, modelName, native.Body)
		return
	}
	applyGeminiTranslation(native)

	// Validate Gemini protocol and schema rules with optional key config
	valRes := validator.ValidateGeminiRequest(modelName, &geminiReq, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		rejectInvalidRequest(c, valRes.Error)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
	}

	canonicalReq := convertInboundGeminiToCanonical(modelName, &geminiReq, isStream)
	canonicalReq.Native = native

	sessionID := resolveSessionID(c, canonicalReq.Model, canonicalReq.Messages, "")
	// The provider call is detached from the downstream request so a customer
	// disconnect cannot cancel it before its usage report arrives.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionAffinityKey(c, sessionID))
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()

	if !isStream {
		h.handleGeminiUnary(c, reqCtx, canonicalReq, modelName, sessionID, start)
		return
	}

	h.handleGeminiStream(c, reqCtx, canonicalReq, modelName, sessionID, start)
}

func (h *Handler) handleGeminiUnary(c *gin.Context, reqCtx context.Context, canonicalReq *model.ChatCompletionRequest, modelName, sessionID string, start time.Time) {
	if !topUpQuotaForRequest(c, canonicalReq) {
		return
	}
	resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
	if err != nil {
		statusCode, _, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondGeminiError(c, statusCode, sanitizeErrorMessage(err), geminiErrorStatus(statusCode))
		return
	}

	geminiResp := convertCanonicalToGeminiResponse(resp)
	dur := time.Since(start)
	pTokens, cTokens, cachedTokens, usageSource := resolveChatUsage(canonicalReq, resp)
	RecordUsage(c, AuditRecordParams{
		SessionID:        sessionID,
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		Model:            modelName,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		CachedTokens:     cachedTokens,
		Duration:         dur,
		TTFT:             dur,
		StatusCode:       http.StatusOK,
		UsageSource:      usageSource,
	})
	if len(resp.NativeResponse) > 0 {
		if err := h.retainGeneratedFiles(c, modelName, resp.NativeIdentity, resp.NativeResponse); err != nil {
			RespondGeminiError(c, 503, "Cannot retain generated media ownership", "UNAVAILABLE")
			return
		}
		c.Data(http.StatusOK, "application/json", resp.NativeResponse)
		return
	}
	c.JSON(http.StatusOK, geminiResp)
}

func convertInboundGeminiToCanonical(modelName string, geminiReq *provider.GeminiRequest, stream bool) *model.ChatCompletionRequest {
	req, err := adapter.GeminiToCanonical(geminiReq)
	if err != nil {
		return &model.ChatCompletionRequest{Model: modelName, Stream: stream}
	}
	req.Model = modelName
	req.Stream = stream
	return req
}

func convertCanonicalToGeminiResponse(resp *model.ChatCompletionResponse) any {
	out, err := adapter.GeminiFromCanonical(resp)
	if err != nil {
		return gin.H{
			"candidates": []gin.H{},
		}
	}
	return out
}

// geminiErrorStatus maps an HTTP status onto the Google API error status string.
func geminiErrorStatus(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusInternalServerError:
		return "INTERNAL"
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return "INVALID_ARGUMENT"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusPaymentRequired, http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusGatewayTimeout:
		return "DEADLINE_EXCEEDED"
	case statusClientClosed:
		return "CANCELLED"
	default:
		return "UNAVAILABLE"
	}
}
