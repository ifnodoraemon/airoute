package api

import (
	"bytes"
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

// maxMultimodalResponseBytes bounds how much of an upstream modality response is
// buffered before it is billed and written. Modality payloads (image JSON, audio
// files, video task JSON) are small; 64 MiB leaves ample room while keeping a
// runaway or malformed upstream from consuming unbounded memory.
const maxMultimodalResponseBytes int64 = 64 << 20

// readMultimodalBody reads the whole upstream body up to limit+1 bytes so a
// truncated/failed read is reported before anything is charged or written.
func readMultimodalBody(stream io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("upstream response exceeds %d bytes", limit)
	}
	return body, nil
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
	// The provider call is detached from the downstream request (and bounded by
	// the drain timeout) exactly like the chat handlers: a customer that hangs up
	// must not cancel a generation the provider has already started, or the
	// gateway absorbs a cost it cannot bill.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()

	resp, err := h.dispatcher.DispatchHTTP(reqCtx, upReq)
	if err != nil {
		// A cancelled dispatch means the customer went away, not that the provider
		// failed: report and audit 499 instead of blaming the upstream with the
		// blanket 502 these endpoints used to return. Other dispatch failures keep
		// the established 502 contract, now with the reason and channel in the
		// audit row.
		if resolved, _, _, _ := resolveUpstreamFailureStatus(err); resolved == statusClientClosed {
			respondUpstreamDispatchError(c, sessionID, upReq.Model, time.Since(start), err)
			return
		}
		recordFailedRequestWithError(c, sessionID, upReq.Model, time.Since(start), http.StatusBadGateway, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": sanitizeErrorMessage(err)})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	succeeded := resp.StatusCode >= 200 && resp.StatusCode < 300

	// Persist (and rewrite) the upstream response BEFORE charging. Charging first
	// meant a task that could not be stored was still billed while the client got
	// a 502 and no task id to poll, so a retry paid twice for one job.
	if succeeded && transform != nil {
		if err := transform(resp); err != nil {
			recordFailedRequestWithError(c, sessionID, upReq.Model, dur, http.StatusBadGateway, err)
			RespondOpenAIError(c, http.StatusBadGateway, "Unable to persist or read video task", "upstream_error", "video_task_error")
			return
		}
	}

	// Read (and buffer) the upstream body BEFORE charging or writing the
	// response head. The old flow billed and flushed HTTP 200 first, then copied
	// the stream and ignored the copy error: a truncated upstream response was
	// charged as a success while the client received a broken 200. The body is
	// also what the video transform may already have replaced with its rewritten
	// JSON, so reading here keeps the persisted task id path intact.
	if succeeded {
		body, readErr := readMultimodalBody(resp.Stream, maxMultimodalResponseBytes)
		if readErr != nil {
			recordFailedRequestWithError(c, sessionID, upReq.Model, dur, http.StatusBadGateway, readErr)
			RespondOpenAIError(c, http.StatusBadGateway, "Unable to read upstream response", "upstream_error", "upstream_read_error")
			return
		}
		resp.Stream = io.NopCloser(bytes.NewReader(body))
	}

	if succeeded {
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
