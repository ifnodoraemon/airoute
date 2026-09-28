package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// HandleRerank handles cross-encoder rerank POST /v1/rerank.
func (h *Handler) HandleRerank(c *gin.Context) {
	var req model.RerankRequest
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

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'query' field in request body",
				"type":    "invalid_request_error",
				"code":    "missing_query",
			},
		})
		return
	}

	if len(req.Documents) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'documents' field in request body or documents list is empty",
				"type":    "invalid_request_error",
				"code":    "missing_documents",
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

	start := time.Now()
	resp, err := h.dispatcher.DispatchRerank(c.Request.Context(), &req)
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

	sessionID := c.GetHeader("X-Session-ID")
	if sessionID == "" {
		sessionID = c.GetHeader("X-Airoute-Session-ID")
	}
	if sessionID == "" {
		sessionID = c.GetHeader("X-Nano-Session-ID")
	}
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_rrk_%d_%x", time.Now().Unix(), time.Now().UnixNano()%1000000)
	}
	c.Header("X-Airoute-Session-ID", sessionID)
	c.Header("X-Nano-Session-ID", sessionID)

	dur := time.Since(start)
	var cost float64
	if billing.GlobalEngine != nil {
		cost, _ = billing.GlobalEngine.CalculateCostWithGroup(req.Model, getKeyGroup(c), resp.Usage.TotalTokens, 0, 0)
	}
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:          middleware.GetTraceID(c),
			SessionID:        sessionID,
			APIKey:           getRequestAPIKey(c),
			TenantID:         c.GetString("tenant_id"),
			Model:            req.Model,
			PromptTokens:     resp.Usage.TotalTokens,
			CompletionTokens: 0,
			TotalTokens:      resp.Usage.TotalTokens,
			Cost:             cost,
			DurationMs:       dur.Milliseconds(),
			StatusCode:       http.StatusOK,
		})
	}

	c.JSON(http.StatusOK, resp)
}
