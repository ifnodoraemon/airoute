package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// HandleRerank handles cross-encoder rerank POST /v1/rerank.
func (h *Handler) HandleRerank(c *gin.Context) {
	var req model.RerankRequest
	native, err := bindNativeJSON(c, &req, model.NativeRerank)
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

	if req.Query == "" {
		RespondOpenAIError(c, http.StatusBadRequest, "Missing 'query' field in request body", "invalid_request_error", "missing_query")
		return
	}

	if len(req.Documents) == 0 {
		RespondOpenAIError(c, http.StatusBadRequest, "Missing 'documents' field in request body or documents list is empty", "invalid_request_error", "missing_documents")
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
	sessionID := resolveGenericSessionID(c, "rrk")

	if !requireQuotaEstimate(c, req.Model, req.EstimatedPromptTokens(), 0, 1) {
		return
	}

	resp, err := h.dispatcher.DispatchRerank(c.Request.Context(), &req)
	if err != nil {
		statusCode, errType, errCode, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, req.Model, time.Since(start), statusCode, err)
		RespondOpenAIError(c, statusCode, sanitizeErrorMessage(err), errType, errCode)
		return
	}

	tokens, usageSource := resolveRerankTokens(&req, resp.Usage)
	RecordUsage(c, AuditRecordParams{
		SessionID:    sessionID,
		Channel:      resp.Channel,
		Model:        req.Model,
		PromptTokens: tokens,
		TotalTokens:  tokens,
		Duration:     time.Since(start),
		StatusCode:   http.StatusOK,
		UsageSource:  usageSource,
	})

	if len(resp.NativeResponse) > 0 {
		c.Data(http.StatusOK, "application/json", resp.NativeResponse)
		return
	}
	c.JSON(http.StatusOK, resp)
}
