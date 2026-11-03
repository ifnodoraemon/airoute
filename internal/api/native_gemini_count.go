package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

func (h *Handler) handleGeminiCount(c *gin.Context, name string, body []byte) {
	if h.handleNativeGeminiCount(c, name, body) {
		return
	}
	body = provider.CanonicalGeminiJSON(body)
	var wrapper struct {
		Request json.RawMessage `json:"generateContentRequest"`
	}
	_ = json.Unmarshal(body, &wrapper)
	if len(wrapper.Request) > 0 {
		body = wrapper.Request
	}
	var request provider.GeminiRequest
	if err := json.Unmarshal(body, &request); err != nil {
		RespondGeminiError(c, http.StatusBadRequest, "Invalid token counting request", "INVALID_ARGUMENT")
		return
	}
	result := validator.ValidateGeminiRequest(name, &request, getAPIKeyConfig(c))
	if !result.Valid && result.Error != nil {
		rejectInvalidRequest(c, result.Error)
		return
	}
	if len(result.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(result.Warnings, "; "))
	}
	chars := 0
	if request.SystemInstruction != nil {
		for _, part := range request.SystemInstruction.Parts {
			chars += len(part.Text)
		}
	}
	for _, content := range request.Contents {
		for _, part := range content.Parts {
			chars += len(part.Text)
		}
	}
	c.Header("X-Airoute-Token-Estimate", "true")
	c.JSON(http.StatusOK, gin.H{"totalTokens": max(1, model.EstimateTokensFromChars(chars))})
}

func (h *Handler) handleNativeGeminiCount(c *gin.Context, modelName string, body []byte) bool {
	found := false
	for _, channel := range h.dispatcher.ListChannelsForModel(modelName, "") {
		found = found || model.IsGeminiProvider(channel.Type)
	}
	if !found {
		return false
	}
	resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), &router.UpstreamRequest{
		Model: modelName, Method: http.MethodPost, ContentType: "application/json", Body: body, GeminiAction: "countTokens",
	})
	if err != nil {
		setUpstreamRetryAfter(c, err)
		status, _, _, _ := resolveUpstreamFailureStatus(err)
		RespondGeminiError(c, status, sanitizeErrorMessage(err), geminiErrorStatus(status))
		return true
	}
	defer resp.Stream.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Stream, 1<<20))
	if err != nil {
		RespondGeminiError(c, http.StatusBadGateway, "Failed to read upstream token count", "UNAVAILABLE")
		return true
	}
	c.Data(resp.StatusCode, "application/json", data)
	return true
}
