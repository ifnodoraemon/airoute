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
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, fmt.Sprintf("Invalid JSON request body: %v", err), "invalid_request_error", "invalid_payload")
		return
	}

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

	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model), "forbidden", "model_not_allowed")
		return
	}

	start := time.Now()
	sessionID := resolveGenericSessionID(c, "rrk")

	resp, err := h.dispatcher.DispatchRerank(c.Request.Context(), &req)
	if err != nil {
		recordFailedRequest(c, sessionID, req.Model, time.Since(start), http.StatusBadGateway)
		RespondOpenAIError(c, http.StatusBadGateway, err.Error(), "gateway_error", "upstream_failure")
		return
	}

	RecordUsage(c, AuditRecordParams{
		SessionID:    sessionID,
		Channel:      resp.Channel,
		Model:        req.Model,
		PromptTokens: resp.Usage.TotalTokens,
		TotalTokens:  resp.Usage.TotalTokens,
		Duration:     time.Since(start),
		StatusCode:   http.StatusOK,
	})

	c.JSON(http.StatusOK, resp)
}
