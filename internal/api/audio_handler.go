package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// HandleAudioSpeech handles text-to-speech POST /v1/audio/speech.
func (h *MultimodalHandler) HandleAudioSpeech(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	var req model.AudioSpeechRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, fmt.Sprintf("Invalid JSON request: %v", err), "invalid_request_error", "invalid_json")
		return
	}

	if req.Model == "" {
		req.Model = "tts-1"
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed for your API key", req.Model), "forbidden", "model_not_allowed")
		return
	}

	sessionID := resolveGenericSessionID(c, "tts")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:        "/v1/audio/speech",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: "application/json",
		Model:       req.Model,
		Protocol:    "audio_speech",
	}, sessionID, "audio/mpeg")
}

// HandleAudioTranscriptions handles speech-to-text POST /v1/audio/transcriptions.
func (h *MultimodalHandler) HandleAudioTranscriptions(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Failed to read multipart body", "invalid_request_error", "bad_request")
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	modelName := c.PostForm("model")
	if modelName == "" {
		modelName = "whisper-1"
	}

	if !middleware.ValidateModelAllowed(c, modelName) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed for your API key", modelName), "forbidden", "model_not_allowed")
		return
	}

	contentType := c.GetHeader("Content-Type")
	if contentType == "" {
		contentType = c.ContentType()
	}
	if contentType == "" {
		contentType = "multipart/form-data"
	}

	sessionID := resolveGenericSessionID(c, "stt")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:        "/v1/audio/transcriptions",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: contentType,
		Model:       modelName,
		Protocol:    "audio_transcription",
	}, sessionID)
}

// HandleAudioTranslations handles audio translation POST /v1/audio/translations.
func (h *MultimodalHandler) HandleAudioTranslations(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Failed to read multipart body", "invalid_request_error", "bad_request")
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	modelName := c.PostForm("model")
	if modelName == "" {
		modelName = "whisper-1"
	}

	if !middleware.ValidateModelAllowed(c, modelName) {
		RespondOpenAIError(c, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed for your API key", modelName), "forbidden", "model_not_allowed")
		return
	}

	contentType := c.GetHeader("Content-Type")
	if contentType == "" {
		contentType = c.ContentType()
	}
	if contentType == "" {
		contentType = "multipart/form-data"
	}

	sessionID := resolveGenericSessionID(c, "stt")
	h.dispatchAndForward(c, &router.UpstreamRequest{
		Path:        "/v1/audio/translations",
		Method:      http.MethodPost,
		Body:        bodyBytes,
		ContentType: contentType,
		Model:       modelName,
		Protocol:    "audio_transcription",
	}, sessionID)
}
