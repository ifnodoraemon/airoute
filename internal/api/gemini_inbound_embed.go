package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// geminiEmbedContentRequest is the documented :embedContent payload. It is not a
// GeminiRequest: embeddings carry a single `content` and optional task metadata,
// with no generationConfig/contents/candidates.
type geminiEmbedContentRequest struct {
	Model   string `json:"model"`
	Content struct {
		Parts []provider.GeminiPart `json:"parts"`
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
	native, err := bindNativeJSON(c, &payload, model.NativeGeminiEmbedding)
	if err != nil {
		respondGeminiEmbedParseError(c, err)
		return
	}

	text := payload.geminiContentText()
	if len(payload.Content.Parts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    400,
				"message": "embedContent requires content.parts",
				"status":  "INVALID_ARGUMENT",
			},
		})
		return
	}

	native.TranslationError = geminiEmbeddingTranslationError(native.Body)
	embReq := &model.EmbeddingRequest{Input: text, Model: modelName, Native: native}
	if payload.OutputDimensionality != nil && *payload.OutputDimensionality > 0 {
		embReq.Dimensions = payload.OutputDimensionality
	}

	start := time.Now()
	sessionID := resolveGenericSessionID(c, "emb")

	if !requireQuotaEstimate(c, modelName, embReq.EstimatedPromptTokens(), 0, 1) {
		return
	}
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

	renderGeminiEmbedding(c, resp)
}

func respondGeminiEmbedParseError(c *gin.Context, err error) {
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
}

func renderGeminiEmbedding(c *gin.Context, resp *model.EmbeddingResponse) {
	values := []float64{}
	if len(resp.NativeResponse) > 0 {
		c.Data(http.StatusOK, "application/json", resp.NativeResponse)
		return
	}
	if len(resp.Data) > 0 {
		values = resp.Data[0].Embedding
	}
	c.JSON(http.StatusOK, gin.H{
		"embedding": gin.H{"values": values},
	})
}
