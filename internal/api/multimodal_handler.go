package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/billing"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
)

func resolveMMSessionID(c *gin.Context, prefix string) string {
	sID := c.GetHeader("X-Session-ID")
	if sID == "" {
		sID = c.GetHeader("X-Nano-Session-ID")
	}
	if sID == "" {
		sID = fmt.Sprintf("sess_%s_%d_%x", prefix, time.Now().Unix(), time.Now().UnixNano()%1000000)
	}
	c.Header("X-Nano-Session-ID", sID)
	return sID
}

// MultimodalHandler handles image, audio (TTS/STT), and video modalities.
type MultimodalHandler struct {
	dispatcher *router.Dispatcher
}

// NewMultimodalHandler creates a MultimodalHandler.
func NewMultimodalHandler(dispatcher *router.Dispatcher) *MultimodalHandler {
	return &MultimodalHandler{dispatcher: dispatcher}
}

// HandleImageGenerations handles POST /v1/images/generations.
func (h *MultimodalHandler) HandleImageGenerations(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req model.ImageGenerationRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON request: %v", err)})
		return
	}

	if req.Model == "" {
		req.Model = "dall-e-3"
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	sessionID := resolveMMSessionID(c, "img")

	start := time.Now()
	upReq := &router.UpstreamRequest{
		Path:        "/v1/images/generations",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: "application/json",
		Model:       req.Model,
		Protocol:    "images",
	}

	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	var cost float64
	if billing.GlobalEngine != nil {
		cost, _ = billing.GlobalEngine.CalculateCostWithGroup(req.Model, getKeyGroup(c), 0, 0, 0)
	}
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:    middleware.GetTraceID(c),
			SessionID:  sessionID,
			VirtualKey: c.GetString("virtual_key"),
			TenantID:   c.GetString("tenant_id"),
			Model:      req.Model,
			Cost:       cost,
			DurationMs: dur.Milliseconds(),
			StatusCode: resp.StatusCode,
		})
	}

	for k, vals := range resp.Headers {
		for _, v := range vals {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Stream)
}

// HandleAudioSpeech handles text-to-speech POST /v1/audio/speech.
func (h *MultimodalHandler) HandleAudioSpeech(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req model.AudioSpeechRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON request: %v", err)})
		return
	}

	if req.Model == "" {
		req.Model = "tts-1"
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	sessionID := resolveMMSessionID(c, "tts")

	start := time.Now()
	upReq := &router.UpstreamRequest{
		Path:        "/v1/audio/speech",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: "application/json",
		Model:       req.Model,
		Protocol:    "audio_speech",
	}

	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	var cost float64
	if billing.GlobalEngine != nil {
		cost, _ = billing.GlobalEngine.CalculateCostWithGroup(req.Model, getKeyGroup(c), 0, 0, 0)
	}
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:    middleware.GetTraceID(c),
			SessionID:  sessionID,
			VirtualKey: c.GetString("virtual_key"),
			TenantID:   c.GetString("tenant_id"),
			Model:      req.Model,
			Cost:       cost,
			DurationMs: dur.Milliseconds(),
			StatusCode: resp.StatusCode,
		})
	}

	// Stream audio binary directly to downstream client
	contentType := resp.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	c.Header("Content-Type", contentType)
	c.Status(resp.StatusCode)

	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.Copy(c.Writer, resp.Stream)
}

// HandleAudioTranscriptions handles speech-to-text POST /v1/audio/transcriptions.
func (h *MultimodalHandler) HandleAudioTranscriptions(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read multipart body"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Read multipart form to identify the requested model
	modelName := c.PostForm("model")
	if modelName == "" {
		modelName = "whisper-1"
	}

	if !middleware.ValidateModelAllowed(c, modelName) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' is not allowed for your API key", modelName),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	start := time.Now()
	upReq := &router.UpstreamRequest{
		Path:        "/v1/audio/transcriptions",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: c.ContentType(),
		Model:       modelName,
		Protocol:    "audio_transcription",
	}

	sessionID := resolveMMSessionID(c, "stt")

	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	var cost float64
	if billing.GlobalEngine != nil {
		cost, _ = billing.GlobalEngine.CalculateCostWithGroup(modelName, getKeyGroup(c), 0, 0, 0)
	}
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:    middleware.GetTraceID(c),
			SessionID:  sessionID,
			VirtualKey: c.GetString("virtual_key"),
			TenantID:   c.GetString("tenant_id"),
			Model:      modelName,
			Cost:       cost,
			DurationMs: dur.Milliseconds(),
			StatusCode: resp.StatusCode,
		})
	}

	for k, vals := range resp.Headers {
		for _, v := range vals {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Stream)
}

// HandleAudioTranslations handles audio translation POST /v1/audio/translations.
func (h *MultimodalHandler) HandleAudioTranslations(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read multipart body"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	modelName := c.PostForm("model")
	if modelName == "" {
		modelName = "whisper-1"
	}

	if !middleware.ValidateModelAllowed(c, modelName) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' is not allowed for your API key", modelName),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	sessionID := resolveMMSessionID(c, "stt")

	start := time.Now()
	upReq := &router.UpstreamRequest{
		Path:        "/v1/audio/translations",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: c.ContentType(),
		Model:       modelName,
		Protocol:    "audio_transcription",
	}

	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	var cost float64
	if billing.GlobalEngine != nil {
		cost, _ = billing.GlobalEngine.CalculateCostWithGroup(modelName, getKeyGroup(c), 0, 0, 0)
	}
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:    middleware.GetTraceID(c),
			SessionID:  sessionID,
			VirtualKey: c.GetString("virtual_key"),
			TenantID:   c.GetString("tenant_id"),
			Model:      modelName,
			Cost:       cost,
			DurationMs: dur.Milliseconds(),
			StatusCode: resp.StatusCode,
		})
	}

	for k, vals := range resp.Headers {
		for _, v := range vals {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Stream)
}

// HandleVideoGenerations handles text-to-video POST /v1/videos/generations.
func (h *MultimodalHandler) HandleVideoGenerations(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req model.VideoGenerationRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON request: %v", err)})
		return
	}

	if req.Model == "" {
		req.Model = "cogvideox"
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	sessionID := resolveMMSessionID(c, "vid")

	start := time.Now()
	upReq := &router.UpstreamRequest{
		Path:        "/v1/videos/generations",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: "application/json",
		Model:       req.Model,
		Protocol:    "videos",
	}

	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	dur := time.Since(start)
	var cost float64
	if billing.GlobalEngine != nil {
		cost, _ = billing.GlobalEngine.CalculateCostWithGroup(req.Model, getKeyGroup(c), 0, 0, 0)
	}
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:    middleware.GetTraceID(c),
			SessionID:  sessionID,
			VirtualKey: c.GetString("virtual_key"),
			TenantID:   c.GetString("tenant_id"),
			Model:      req.Model,
			Cost:       cost,
			DurationMs: dur.Milliseconds(),
			StatusCode: resp.StatusCode,
		})
	}

	for k, vals := range resp.Headers {
		for _, v := range vals {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Stream)
}

// HandleVideoTask handles polling video status GET /v1/videos/tasks/:id.
func (h *MultimodalHandler) HandleVideoTask(c *gin.Context) {
	taskID := c.Param("id")
	modelName := c.Query("model")
	if modelName == "" {
		modelName = "cogvideox"
	}

	upReq := &router.UpstreamRequest{
		Path:        "/v1/videos/tasks/" + taskID,
		Method:      http.MethodGet,
		ContentType: "application/json",
		Model:       modelName,
		Protocol:    "videos",
	}

	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), upReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Stream.Close()

	for k, vals := range resp.Headers {
		for _, v := range vals {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Stream)
}
