package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	CaptureRequestSnapshot(c)
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
	canonicalReq.AnthropicBeta = c.GetHeader("anthropic-beta")

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
	if len(resp.NativeAnthropicResponse) > 0 {
		var native map[string]any
		if err := json.Unmarshal(resp.NativeAnthropicResponse, &native); err == nil {
			native["model"] = req.Model
			inputTokens, outputTokens := 0, 0
			if resp.Usage != nil {
				inputTokens, outputTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens
			}
			RecordUsage(c, AuditRecordParams{SessionID: sessionID, ChatID: resp.ID, Channel: resp.Channel, Model: req.Model, PromptTokens: inputTokens, CompletionTokens: outputTokens, Duration: time.Since(start), StatusCode: http.StatusOK})
			c.JSON(http.StatusOK, native)
			return
		}
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
	cachedTokens := 0
	if resp.Usage != nil {
		cachedTokens = resp.Usage.GetCachedTokens()
		// Anthropic's input_tokens excludes prompt-cache reads; our canonical
		// PromptTokens includes them, so subtract before rendering the response.
		inputTokens = resp.Usage.PromptTokens - cachedTokens
		if inputTokens < 0 {
			inputTokens = 0
		}
		outputTokens = resp.Usage.CompletionTokens
	}

	usagePayload := gin.H{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	}
	if cachedTokens > 0 {
		usagePayload["cache_read_input_tokens"] = cachedTokens
	}

	anthropicResp := gin.H{
		"id":          fmt.Sprintf("msg_%s", resp.ID),
		"type":        "message",
		"role":        "assistant",
		"model":       req.Model,
		"content":     contentBlocks,
		"stop_reason": stopReason,
		"usage":       usagePayload,
	}

	RecordUsage(c, AuditRecordParams{
		SessionID:        sessionID,
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		Model:            req.Model,
		PromptTokens:     inputTokens + cachedTokens,
		CompletionTokens: outputTokens,
		CachedTokens:     cachedTokens,
		Duration:         time.Since(start),
		StatusCode:       http.StatusOK,
		UsageSource:      "upstream",
	})

	c.JSON(http.StatusOK, anthropicResp)
}

// HandleAnthropicCountTokens delegates token counting to the upstream Anthropic
// tokenizer when a native channel serves the model. Otherwise it returns a local
// character-based estimate (flagged with X-Airoute-Token-Estimate) so that
// Anthropic SDK clients keep working on OpenAI/Gemini-backed models.
func (h *Handler) HandleAnthropicCountTokens(c *gin.Context) {
	var req model.AnthropicInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondAnthropicError(c, http.StatusBadRequest, "invalid request body", "invalid_request_error")
		return
	}
	if req.Model == "" || len(req.Messages) == 0 {
		RespondAnthropicError(c, http.StatusBadRequest, "model and messages are required", "invalid_request_error")
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondAnthropicError(c, http.StatusForbidden, "model is not permitted for this API key", "permission_error")
		return
	}
	channels := h.dispatcher.ResolveCandidateChannels(c.Request.Context(), req.Model, "anthropic_messages")
	var lastErr error
	for _, channel := range channels {
		if channel.Type != model.ProviderAnthropic {
			continue
		}
		payload := map[string]any{"model": channel.GetUpstreamModel(req.Model), "messages": req.Messages}
		if req.System != nil {
			payload["system"] = req.System
		}
		if len(req.Tools) > 0 {
			payload["tools"] = req.Tools
		}
		if req.ToolChoice != nil {
			payload["tool_choice"] = req.ToolChoice
		}
		if req.Thinking != nil {
			payload["thinking"] = req.Thinking
		}
		body, err := json.Marshal(payload)
		if err != nil {
			lastErr = err
			continue
		}
		url := strings.TrimRight(channel.BaseURL, "/")
		if strings.HasSuffix(url, "/v1/messages") {
			url += "/count_tokens"
		} else {
			url += "/v1/messages/count_tokens"
		}
		httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", channel.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		if beta := c.GetHeader("anthropic-beta"); beta != "" {
			httpReq.Header.Set("anthropic-beta", beta)
		}
		resp, err := provider.SharedDefaultHTTPClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("channel %s returned HTTP %d: %s", channel.Name, resp.StatusCode, responseBody)
			continue
		}
		var result struct {
			InputTokens int `json:"input_tokens"`
		}
		if err := json.Unmarshal(responseBody, &result); err != nil {
			lastErr = err
			continue
		}
		c.JSON(http.StatusOK, result)
		return
	}
	if lastErr == nil {
		// No native Anthropic channel serves this model (it is routed to an
		// OpenAI/Gemini-compatible upstream). Anthropic SDK clients call
		// /messages/count_tokens routinely before every turn, so returning 501
		// here would break otherwise working clients. Fall back to a local
		// character-based estimate and mark the response as approximate.
		c.Header("X-Airoute-Token-Estimate", "true")
		c.JSON(http.StatusOK, gin.H{"input_tokens": estimateAnthropicInputTokens(&req)})
		return
	}
	RespondAnthropicError(c, http.StatusBadGateway, lastErr.Error(), "api_error")
}

// estimateAnthropicInputTokens provides a conservative local token estimate
// (roughly 4 bytes per token) for models without a native Anthropic tokenizer.
func estimateAnthropicInputTokens(req *model.AnthropicInboundRequest) int {
	if req == nil {
		return 1
	}
	chars := len(model.ExtractAnthropicContentString(req.System))
	for _, m := range req.Messages {
		chars += len(model.ExtractAnthropicContentString(m.Content))
	}
	if len(req.Tools) > 0 {
		if b, err := json.Marshal(req.Tools); err == nil {
			chars += len(b)
		}
	}
	tokens := chars / 4
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}
