package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// MultimodalHandler handles image, audio (TTS/STT), and video modalities.
type MultimodalHandler struct {
	dispatcher *router.Dispatcher
}

// NewMultimodalHandler creates a MultimodalHandler.
func NewMultimodalHandler(dispatcher *router.Dispatcher) *MultimodalHandler {
	return &MultimodalHandler{dispatcher: dispatcher}
}

// dispatchAndForward executes an UpstreamRequest via dispatcher, audits the usage,
// and proxies headers + stream back to the downstream client (Template Method Pattern).
func (h *MultimodalHandler) dispatchAndForward(c *gin.Context, upReq *router.UpstreamRequest, sessionID string, defaultContentType ...string) {
	start := time.Now()
	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		recordFailedRequest(c, sessionID, upReq.Model, time.Since(start), http.StatusBadGateway)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		RecordUsage(c, AuditRecordParams{
			SessionID:  sessionID,
			Channel:    resp.Channel,
			Model:      upReq.Model,
			Duration:   dur,
			StatusCode: resp.StatusCode,
		})
	} else {
		recordFailedRequest(c, sessionID, upReq.Model, dur, resp.StatusCode)
	}

	for k, vals := range resp.Headers {
		for _, v := range vals {
			c.Header(k, v)
		}
	}

	if len(defaultContentType) > 0 && defaultContentType[0] != "" {
		if c.Writer.Header().Get("Content-Type") == "" {
			c.Header("Content-Type", defaultContentType[0])
		}
	}

	c.Status(resp.StatusCode)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.Copy(c.Writer, resp.Stream)
}

// HandleImageGenerations handles POST /v1/images/generations.
func (h *MultimodalHandler) HandleImageGenerations(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	var req model.ImageGenerationRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, fmt.Sprintf("Invalid JSON request: %v", err), "invalid_request_error", "invalid_json")
		return
	}

	if req.Model == "" {
		req.Model = "dall-e-3"
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model), "forbidden", "model_not_allowed")
		return
	}

	sessionID := resolveGenericSessionID(c, "img")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:        "/v1/images/generations",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: "application/json",
		Model:       req.Model,
		Protocol:    "images",
	}, sessionID)
}
