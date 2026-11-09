package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// HandleEmbeddings handles POST /v1/embeddings.
func (h *Handler) HandleEmbeddings(c *gin.Context) {
	var req model.EmbeddingRequest
	native, err := bindNativeJSON(c, &req, model.NativeEmbeddings)
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

	if req.Input == nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Missing 'input' field in request body", "invalid_request_error", "missing_input")
		return
	}

	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model), "forbidden", "model_not_allowed")
		return
	}

	start := time.Now()
	sessionID := resolveGenericSessionID(c, "emb")

	if !requireQuotaEstimate(c, req.Model, req.EstimatedPromptTokens(), 0, 1) {
		return
	}

	resp, err := h.dispatcher.DispatchEmbedding(c.Request.Context(), &req)
	if err != nil {
		statusCode, errType, errCode, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, req.Model, time.Since(start), statusCode, err)
		RespondOpenAIError(c, statusCode, sanitizeErrorMessage(err), errType, errCode)
		return
	}

	pTokens, usageSource := resolveEmbeddingTokens(&req, resp.Usage)
	RecordUsage(c, AuditRecordParams{
		SessionID:    sessionID,
		Channel:      resp.Channel,
		Model:        req.Model,
		PromptTokens: pTokens,
		TotalTokens:  pTokens,
		Duration:     time.Since(start),
		TTFT:         time.Since(start),
		StatusCode:   http.StatusOK,
		UsageSource:  usageSource,
	})

	if len(resp.NativeResponse) > 0 {
		c.Data(http.StatusOK, "application/json", resp.NativeResponse)
		return
	}
	c.JSON(http.StatusOK, resp)
}
