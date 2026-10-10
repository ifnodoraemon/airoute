package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/adapter"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

// AnthropicInboundMessage represents an inbound message from Claude SDK.
type AnthropicInboundMessage = model.AnthropicInboundMessage

// AnthropicInboundRequest represents the incoming payload from Anthropic SDK.
type AnthropicInboundRequest = model.AnthropicInboundRequest

// ConvertAnthropicToCanonical converts an Anthropic request to the canonical OpenAI request format via Adapter Pattern.
func ConvertAnthropicToCanonical(req *AnthropicInboundRequest) *model.ChatCompletionRequest {
	ad, ok := adapter.Get("anthropic")
	if !ok {
		ad = adapter.NewAnthropicAdapter()
	}
	res, err := ad.ToCanonical(context.Background(), req)
	if err != nil {
		return &model.ChatCompletionRequest{Model: req.Model}
	}
	return res
}

// HandleAnthropicMessages handles POST /v1/messages for Anthropic SDK clients.
func (h *Handler) HandleAnthropicMessages(c *gin.Context) {
	var req AnthropicInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondAnthropicError(c, http.StatusBadRequest, fmt.Sprintf("Failed to parse Anthropic request: %v", err), "invalid_request_error")
		return
	}

	if req.Model == "" {
		RespondAnthropicError(c, http.StatusBadRequest, "model is required", "invalid_request_error")
		return
	}

	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondAnthropicError(c, http.StatusForbidden, fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model), "permission_error")
		return
	}

	// Validate Anthropic protocol and schema rules with optional key config
	valRes := validator.ValidateAnthropicRequest(&req, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		valRes.Error.WriteGinResponse(c)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()
	canonicalReq := ConvertAnthropicToCanonical(&req)

	sessionID := resolveSessionID(c, canonicalReq.Model, canonicalReq.Messages, "")
	reqCtx := c.Request.Context()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
	}

	if !req.Stream {
		h.handleAnthropicUnary(c, reqCtx, &req, canonicalReq, sessionID, start)
		return
	}

	h.handleAnthropicStream(c, reqCtx, &req, canonicalReq, sessionID, start)
}

func (h *Handler) handleAnthropicUnary(c *gin.Context, reqCtx context.Context, req *AnthropicInboundRequest, canonicalReq *model.ChatCompletionRequest, sessionID string, start time.Time) {
	resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
	if err != nil {
		statusCode := http.StatusBadGateway
		errType := "api_error"
		if strings.Contains(err.Error(), "no upstream provider available") {
			statusCode = http.StatusNotFound
			errType = "not_found_error"
		}
		recordFailedRequest(c, sessionID, canonicalReq.Model, time.Since(start), statusCode)
		c.JSON(statusCode, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": err.Error(),
			},
		})
		return
	}

	var contentBlocks []gin.H
	stopReason := "end_turn"

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if replyText := choice.Message.GetContentString(); replyText != "" {
			contentBlocks = append(contentBlocks, gin.H{
				"type": "text",
				"text": replyText,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			var parsedArgs any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsedArgs); err != nil {
				parsedArgs = map[string]any{}
			}
			contentBlocks = append(contentBlocks, gin.H{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": parsedArgs,
			})
		}

		if len(choice.Message.ToolCalls) > 0 || (choice.FinishReason != nil && *choice.FinishReason == "tool_calls") {
			stopReason = "tool_use"
		} else if choice.FinishReason != nil && *choice.FinishReason == "length" {
			stopReason = "max_tokens"
		}
	}

	inputTokens := 0
	outputTokens := 0
	if resp.Usage != nil {
		inputTokens = resp.Usage.PromptTokens
		outputTokens = resp.Usage.CompletionTokens
	}

	anthropicResp := gin.H{
		"id":          fmt.Sprintf("msg_%s", resp.ID),
		"type":        "message",
		"role":        "assistant",
		"model":       req.Model,
		"content":     contentBlocks,
		"stop_reason": stopReason,
		"usage": gin.H{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	RecordUsage(c, AuditRecordParams{
		SessionID:        sessionID,
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		Model:            req.Model,
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		Duration:         time.Since(start),
		StatusCode:       http.StatusOK,
	})

	c.JSON(http.StatusOK, anthropicResp)
}

// HandleAnthropicCountTokens handles POST /v1/messages/count_tokens for Anthropic Claude SDK compatibility.
func (h *Handler) HandleAnthropicCountTokens(c *gin.Context) {
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
		System any `json:"system,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	chars := 0
	if req.System != nil {
		if s, ok := req.System.(string); ok {
			chars += len(s)
		}
	}
	for _, m := range req.Messages {
		if s, ok := m.Content.(string); ok {
			chars += len(s)
		}
	}
	tokens := chars / 4
	if tokens == 0 {
		tokens = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": tokens,
	})
}
