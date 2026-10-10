package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/adapter"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

// HandleGeminiModels handles GET /v1beta/models.
func (h *Handler) HandleGeminiModels(c *gin.Context) {
	models := h.dispatcher.GetAllSupportedModels()
	type geminiModelInfo struct {
		Name                       string   `json:"name"`
		Version                    string   `json:"version"`
		DisplayName                string   `json:"displayName"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	}

	var list []geminiModelInfo
	for _, m := range models {
		list = append(list, geminiModelInfo{
			Name:                       "models/" + m,
			Version:                    "001",
			DisplayName:                m,
			SupportedGenerationMethods: []string{"generateContent", "countTokens", "embedContent"},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"models": list,
	})
}

// HandleGeminiModelDetail handles GET /v1beta/models/*modelAction.
func (h *Handler) HandleGeminiModelDetail(c *gin.Context) {
	param := strings.TrimPrefix(c.Param("modelAction"), "/")
	modelName := strings.TrimPrefix(param, "models/")

	channels := h.dispatcher.GetChannelsForModel(modelName)
	if len(channels) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    404,
				"message": fmt.Sprintf("models/%s is not found", modelName),
				"status":  "NOT_FOUND",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":                       "models/" + modelName,
		"version":                    "001",
		"displayName":                modelName,
		"supportedGenerationMethods": []string{"generateContent", "countTokens", "embedContent"},
	})
}

// HandleGeminiAction handles POST /v1beta/models/*modelAction for Google Gemini SDKs.
func (h *Handler) HandleGeminiAction(c *gin.Context) {
	param := strings.TrimPrefix(c.Param("modelAction"), "/")
	param = strings.TrimPrefix(param, "models/")

	parts := strings.Split(param, ":")
	modelName := parts[0]
	action := "generateContent"
	if len(parts) > 1 {
		action = parts[1]
	}

	isStream := strings.Contains(action, "streamGenerateContent") || c.Query("alt") == "sse"

	if !middleware.ValidateModelAllowed(c, modelName) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"code":    403,
				"message": fmt.Sprintf("API key not allowed to access model '%s'", modelName),
				"status":  "PERMISSION_DENIED",
			},
		})
		return
	}

	var geminiReq provider.GeminiRequest
	if err := c.ShouldBindJSON(&geminiReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": fmt.Sprintf("Failed to parse Gemini request: %v", err),
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	// Validate Gemini protocol and schema rules with optional key config
	valRes := validator.ValidateGeminiRequest(modelName, &geminiReq, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		valRes.Error.WriteGinResponse(c)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
	}

	if action == "countTokens" {
		totalChars := 0
		if geminiReq.SystemInstruction != nil {
			for _, p := range geminiReq.SystemInstruction.Parts {
				totalChars += len(p.Text)
			}
		}
		for _, cnt := range geminiReq.Contents {
			for _, p := range cnt.Parts {
				totalChars += len(p.Text)
			}
		}
		totalTokens := totalChars / 4
		if totalTokens == 0 {
			totalTokens = 1
		}
		c.JSON(http.StatusOK, gin.H{
			"totalTokens": totalTokens,
		})
		return
	}

	canonicalReq := convertInboundGeminiToCanonical(modelName, &geminiReq, isStream)

	sessionID := resolveSessionID(c, canonicalReq.Model, canonicalReq.Messages, "")
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()

	if !isStream {
		h.handleGeminiUnary(c, reqCtx, canonicalReq, modelName, sessionID, start)
		return
	}

	h.handleGeminiStream(c, reqCtx, canonicalReq, modelName, sessionID, start)
}

func (h *Handler) handleGeminiUnary(c *gin.Context, reqCtx context.Context, canonicalReq *model.ChatCompletionRequest, modelName, sessionID string, start time.Time) {
	resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
	if err != nil {
		recordFailedRequest(c, sessionID, canonicalReq.Model, time.Since(start), http.StatusBadGateway)
		RespondGeminiError(c, http.StatusBadGateway, err.Error(), "UNAVAILABLE")
		return
	}

	geminiResp := convertCanonicalToGeminiResponse(resp)
	dur := time.Since(start)
	pTokens := 0
	cTokens := 0
	if resp.Usage != nil {
		pTokens = resp.Usage.PromptTokens
		cTokens = resp.Usage.CompletionTokens
	}
	RecordUsage(c, AuditRecordParams{
		SessionID:        sessionID,
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		Model:            modelName,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		Duration:         dur,
		StatusCode:       http.StatusOK,
	})
	c.JSON(http.StatusOK, geminiResp)
}

func convertInboundGeminiToCanonical(modelName string, geminiReq *provider.GeminiRequest, stream bool) *model.ChatCompletionRequest {
	ad, ok := adapter.Get("gemini")
	if !ok {
		ad = adapter.NewGeminiAdapter()
	}
	req, err := ad.ToCanonical(context.Background(), geminiReq)
	if err != nil {
		return &model.ChatCompletionRequest{Model: modelName, Stream: stream}
	}
	req.Model = modelName
	req.Stream = stream
	return req
}

func convertCanonicalToGeminiResponse(resp *model.ChatCompletionResponse) any {
	ad, ok := adapter.Get("gemini")
	if !ok {
		ad = adapter.NewGeminiAdapter()
	}
	out, err := ad.FromCanonical(context.Background(), resp)
	if err != nil {
		return gin.H{
			"candidates": []gin.H{},
		}
	}
	return out
}
