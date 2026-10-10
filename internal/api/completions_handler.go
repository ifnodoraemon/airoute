package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// LegacyCompletionRequest represents the standard OpenAI text completion payload (POST /v1/completions).
type LegacyCompletionRequest struct {
	Model       string   `json:"model"`
	Prompt      any      `json:"prompt"` // string or []string or []any
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	N           *int     `json:"n,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	User        string   `json:"user,omitempty"`
}

// TextCompletionChoice represents a choice in an OpenAI text completion response.
type TextCompletionChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	Logprobs     any     `json:"logprobs"`
	FinishReason *string `json:"finish_reason"`
}

// TextCompletionResponse represents the response for POST /v1/completions.
type TextCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []TextCompletionChoice `json:"choices"`
	Usage   *model.Usage           `json:"usage,omitempty"`
}

// HandleCompletions handles legacy text completions POST /v1/completions.
func (h *Handler) HandleCompletions(c *gin.Context) {
	var req LegacyCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON request body: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_payload",
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'model' field in request body",
				"type":    "invalid_request_error",
				"code":    "missing_model",
			},
		})
		return
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	var promptText string
	switch v := req.Prompt.(type) {
	case string:
		promptText = v
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		promptText = strings.Join(parts, "\n")
	case []string:
		promptText = strings.Join(v, "\n")
	}

	chatReq := model.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    []model.ChatMessage{{Role: "user", Content: promptText}},
		Temperature: req.Temperature,
		TopP:        req.TopP,
		N:           req.N,
		Stream:      req.Stream,
		User:        req.User,
		Protocol:    "openai_text",
	}
	if req.MaxTokens != nil {
		chatReq.MaxTokens = req.MaxTokens
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	sessionID := resolveSessionID(c, req.Model, chatReq.Messages, req.User)
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	start := time.Now()

	if !req.Stream {
		h.handleCompletionsUnary(c, reqCtx, req, chatReq, sessionID, start)
		return
	}

	h.handleCompletionsStream(c, reqCtx, req, chatReq, sessionID, start)
}

func (h *Handler) handleCompletionsUnary(c *gin.Context, reqCtx context.Context, req LegacyCompletionRequest, chatReq model.ChatCompletionRequest, sessionID string, start time.Time) {
	resp, err := h.dispatcher.Dispatch(reqCtx, &chatReq)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}

	dur := time.Since(start)
	pTokens, cTokens, cachedTokens := 0, 0, 0
	if resp.Usage != nil {
		pTokens = resp.Usage.PromptTokens
		cTokens = resp.Usage.CompletionTokens
		cachedTokens = resp.Usage.GetCachedTokens()
	}
	RecordUsage(c, AuditRecordParams{
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		CachedTokens:     cachedTokens,
		Duration:         dur,
		StatusCode:       http.StatusOK,
	})

	textResp := TextCompletionResponse{
		ID:      resp.ID,
		Object:  "text_completion",
		Created: resp.Created,
		Model:   resp.Model,
		Usage:   resp.Usage,
	}
	for i, ch := range resp.Choices {
		textResp.Choices = append(textResp.Choices, TextCompletionChoice{
			Text:         ch.Message.GetContentString(),
			Index:        i,
			Logprobs:     nil,
			FinishReason: ch.FinishReason,
		})
	}
	c.JSON(http.StatusOK, textResp)
}
