package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// MultimodalHandler handles image, audio (TTS/STT), and video modalities.
type MultimodalHandler struct {
	dispatcher *router.Dispatcher
	repo       *storage.Repository
}

// NewMultimodalHandler creates a MultimodalHandler.
func NewMultimodalHandler(dispatcher *router.Dispatcher, repo *storage.Repository) *MultimodalHandler {
	return &MultimodalHandler{dispatcher: dispatcher, repo: repo}
}

// dispatchAndForward executes an UpstreamRequest via dispatcher, audits the usage,
// and proxies headers + stream back to the downstream client (Template Method Pattern).
func (h *MultimodalHandler) dispatchAndForward(c *gin.Context, upReq *router.UpstreamRequest, sessionID string, defaultContentType ...string) {
	h.dispatchAndTransform(c, upReq, sessionID, nil, defaultContentType...)
}

func (h *MultimodalHandler) dispatchAndTransform(c *gin.Context, upReq *router.UpstreamRequest, sessionID string, transform func(*router.UpstreamResponse) error, defaultContentType ...string) {
	if upReq.Method != http.MethodGet && billing.GlobalEngine != nil && os.Getenv("GATEWAY_ALLOW_UNPRICED_MULTIMODAL") != "1" {
		price := billing.GlobalEngine.GetPriceWithGroup(upReq.Model, getKeyGroup(c))
		if price == nil || price.FixedPrice <= 0 {
			RespondOpenAIError(c, http.StatusUnprocessableEntity, "Configure a positive fixed_price for this multimodal model before use", "invalid_request_error", "pricing_not_configured")
			return
		}
	}
	start := time.Now()
	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		// Keep the routing failure in the audit row: recording only the 502 left
		// operators staring at an error with no message at all.
		recordFailedRequestWithError(c, sessionID, upReq.Model, time.Since(start), http.StatusBadGateway, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": sanitizeErrorMessage(err)})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		RecordUsage(c, AuditRecordParams{
			SessionID:     sessionID,
			Channel:       resp.Channel,
			Model:         upReq.Model,
			Duration:      dur,
			StatusCode:    resp.StatusCode,
			BillableUnits: upReq.BillableUnits,
			SkipBilling:   upReq.Method == http.MethodGet,
		})
	} else {
		// Name the status and the channel that returned it. A bare status code
		// audit row gave operators no way to tell an upstream outage from a
		// missing route.
		recordFailedRequestWithError(c, sessionID, upReq.Model, dur, resp.StatusCode,
			provider.NewUpstreamError(resp.Channel, resp.StatusCode, nil))
	}

	if transform != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := transform(resp); err != nil {
			RespondOpenAIError(c, http.StatusBadGateway, "Unable to persist or read video task", "upstream_error", "video_task_error")
			return
		}
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

	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model), "forbidden", "model_not_allowed")
		return
	}

	sessionID := resolveGenericSessionID(c, "img")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:          "/v1/images/generations",
		Method:        http.MethodPost,
		Body:          bodyBytes,
		ContentType:   "application/json",
		Model:         req.Model,
		Protocol:      "images",
		BillableUnits: max(1, req.N),
	}, sessionID)
}
