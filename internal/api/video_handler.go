package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// HandleVideoGenerations handles text-to-video POST /v1/videos/generations.
func (h *MultimodalHandler) HandleVideoGenerations(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	var req model.VideoGenerationRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, fmt.Sprintf("Invalid JSON request: %v", err), "invalid_request_error", "invalid_json")
		return
	}

	if req.Model == "" {
		req.Model = "cogvideox"
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model), "forbidden", "model_not_allowed")
		return
	}

	sessionID := resolveGenericSessionID(c, "vid")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:        "/v1/videos/generations",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: "application/json",
		Model:       req.Model,
		Protocol:    "videos",
	}, sessionID)
}

// HandleVideoTask handles polling video status GET /v1/videos/tasks/:id.
func (h *MultimodalHandler) HandleVideoTask(c *gin.Context) {
	taskID := c.Param("id")
	modelName := c.Query("model")
	if modelName == "" {
		modelName = "cogvideox"
	}

	sessionID := resolveGenericSessionID(c, "vid")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:        "/v1/videos/tasks/" + taskID,
		Method:      http.MethodGet,
		ContentType: "application/json",
		Model:       modelName,
		Protocol:    "videos",
	}, sessionID)
}
