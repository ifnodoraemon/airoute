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
		// A09: the Gemini discovery endpoints must honour the API key's model
		// allowlist exactly like /v1/models does; otherwise a restricted key can
		// enumerate every model the gateway serves.
		if !middleware.ValidateModelAllowed(c, m) {
			continue
		}
		list = append(list, geminiModelInfo{
			Name:                       "models/" + m,
			Version:                    "001",
			DisplayName:                m,
			SupportedGenerationMethods: geminiSupportedGenerationMethods,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"models": list,
	})
}

// geminiSupportedGenerationMethods lists the methods this gateway actually
// implements for a Gemini model. A08: :embedContent used to be advertised while
// being executed as chat completions; it is now dispatched to the embeddings
// path, so it stays listed — but only because it is implemented.
var geminiSupportedGenerationMethods = []string{"generateContent", "countTokens", "embedContent"}

// HandleGeminiModelDetail handles GET /v1beta/models/*modelAction.
func (h *Handler) HandleGeminiModelDetail(c *gin.Context) {
	param := strings.TrimPrefix(c.Param("modelAction"), "/")
	modelName := strings.TrimPrefix(param, "models/")

	// A09: mirror /v1/models/{model}, which answers 403 for a model outside the
	// key's allowlist.
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

	channels := h.dispatcher.ListChannelsForModel(modelName, "")
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
		"supportedGenerationMethods": geminiSupportedGenerationMethods,
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

	if !validateModelCurrency(c, modelName) {
		return
	}
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

	// A08: :embedContent is a real embeddings call, not a chat completion. It
	// must be parsed and dispatched through the embeddings path; previously the
	// canonical chat converter silently turned it into a generateContent request
	// and the client received chat candidates.
	if action == "embedContent" {
		h.handleGeminiEmbedContent(c, modelName)
		return
	}
	if action != "generateContent" && action != "streamGenerateContent" && action != "countTokens" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": fmt.Sprintf("unsupported Gemini method '%s' for model '%s'", action, modelName),
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	var geminiReq provider.GeminiRequest
	if err := c.ShouldBindJSON(&geminiReq); err != nil {
		if isRequestBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": gin.H{
					"code":    http.StatusRequestEntityTooLarge,
					"message": "Request body is too large for this endpoint. Reduce it or configure GATEWAY_MAX_BODY_BYTES.",
					"status":  "INVALID_ARGUMENT",
				},
			})
			return
		}
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
		rejectInvalidRequest(c, valRes.Error)
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
		totalTokens := model.EstimateTokensFromChars(totalChars)
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
	// The provider call is detached from the downstream request so a customer
	// disconnect cannot cancel it before its usage report arrives.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionAffinityKey(c, sessionID))
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
	topUpQuotaForRequest(c, canonicalReq)
	resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
	if err != nil {
		statusCode, _, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondGeminiError(c, statusCode, sanitizeErrorMessage(err), "UNAVAILABLE")
		return
	}

	geminiResp := convertCanonicalToGeminiResponse(resp)
	dur := time.Since(start)
	pTokens, cTokens, cachedTokens, usageSource := resolveChatUsage(canonicalReq, resp)
	RecordUsage(c, AuditRecordParams{
		SessionID:        sessionID,
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		Model:            modelName,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		CachedTokens:     cachedTokens,
		Duration:         dur,
		TTFT:             dur,
		StatusCode:       http.StatusOK,
		UsageSource:      usageSource,
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

// geminiEmbedContentRequest is the documented :embedContent payload. It is not a
// GeminiRequest: embeddings carry a single `content` and optional task metadata,
// with no generationConfig/contents/candidates.
type geminiEmbedContentRequest struct {
	Model   string `json:"model"`
	Content struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"content"`
	TaskType             string `json:"taskType,omitempty"`
	Title                string `json:"title,omitempty"`
	OutputDimensionality *int   `json:"outputDimensionality,omitempty"`
}

// geminiContentText joins the text parts of an embedContent payload.
func (r *geminiEmbedContentRequest) geminiContentText() string {
	parts := make([]string, 0, len(r.Content.Parts))
	for _, p := range r.Content.Parts {
		if p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// handleGeminiEmbedContent serves POST /v1beta/models/*modelAction:embedContent
// by dispatching to the OpenAI-compatible embeddings path and rendering the
// Gemini embedding response shape.
func (h *Handler) handleGeminiEmbedContent(c *gin.Context, modelName string) {
	var payload geminiEmbedContentRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		if isRequestBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": gin.H{
					"code":    http.StatusRequestEntityTooLarge,
					"message": "Request body is too large for this endpoint. Reduce it or configure GATEWAY_MAX_BODY_BYTES.",
					"status":  "INVALID_ARGUMENT",
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": fmt.Sprintf("Failed to parse embedContent request: %v", err),
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	text := payload.geminiContentText()
	if strings.TrimSpace(text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": "embedContent requires content.parts[].text",
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	embReq := &model.EmbeddingRequest{Input: text, Model: modelName}
	if payload.OutputDimensionality != nil && *payload.OutputDimensionality > 0 {
		embReq.Dimensions = payload.OutputDimensionality
	}

	start := time.Now()
	sessionID := resolveGenericSessionID(c, "emb")

	resp, err := h.dispatcher.DispatchEmbedding(c.Request.Context(), embReq)
	if err != nil {
		statusCode, _, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, modelName, time.Since(start), statusCode, err)
		RespondGeminiError(c, statusCode, sanitizeErrorMessage(err), geminiErrorStatus(statusCode))
		return
	}

	pTokens, usageSource := resolveEmbeddingTokens(embReq, resp.Usage)
	RecordUsage(c, AuditRecordParams{
		SessionID:    sessionID,
		Channel:      resp.Channel,
		Model:        modelName,
		PromptTokens: pTokens,
		TotalTokens:  pTokens,
		Duration:     time.Since(start),
		TTFT:         time.Since(start),
		StatusCode:   http.StatusOK,
		UsageSource:  usageSource,
	})

	values := []float64{}
	if len(resp.Data) > 0 {
		values = resp.Data[0].Embedding
	}
	c.JSON(http.StatusOK, gin.H{
		"embedding": gin.H{"values": values},
	})
}

// geminiErrorStatus maps an HTTP status onto the Google API error status string.
func geminiErrorStatus(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return "INVALID_ARGUMENT"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusGatewayTimeout:
		return "DEADLINE_EXCEEDED"
	case statusClientClosed:
		return "CANCELLED"
	default:
		return "UNAVAILABLE"
	}
}
