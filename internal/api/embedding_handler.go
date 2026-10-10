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
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, fmt.Sprintf("Invalid JSON request body: %v", err), "invalid_request_error", "invalid_payload")
		return
	}

	if req.Model == "" {
		RespondOpenAIError(c, http.StatusBadRequest, "Missing 'model' field in request body", "invalid_request_error", "missing_model")
		return
	}

	if req.Input == nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Missing 'input' field in request body", "invalid_request_error", "missing_input")
		return
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model), "forbidden", "model_not_allowed")
		return
	}

	start := time.Now()
	sessionID := resolveGenericSessionID(c, "emb")

	resp, err := h.dispatcher.DispatchEmbedding(c.Request.Context(), &req)
	if err != nil {
		recordFailedRequest(c, sessionID, req.Model, time.Since(start), http.StatusBadGateway)
		RespondOpenAIError(c, http.StatusBadGateway, err.Error(), "gateway_error", "upstream_failure")
		return
	}

	RecordUsage(c, AuditRecordParams{
		SessionID:    sessionID,
		Channel:      resp.Channel,
		Model:        req.Model,
		PromptTokens: resp.Usage.PromptTokens,
		TotalTokens:  resp.Usage.TotalTokens,
		Duration:     time.Since(start),
		StatusCode:   http.StatusOK,
	})

	c.JSON(http.StatusOK, resp)
}
