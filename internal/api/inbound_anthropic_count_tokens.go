package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// Anthropic /messages/count_tokens handler and its local estimate fallback,
// extracted from inbound_anthropic.go to keep the messages handler under the
// size red line.

func (h *Handler) HandleAnthropicCountTokens(c *gin.Context) {
	var req model.AnthropicInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isRequestBodyTooLarge(err) {
			RespondAnthropicError(c, http.StatusRequestEntityTooLarge,
				"Request body is too large for this endpoint. Reduce it or configure GATEWAY_MAX_BODY_BYTES.", "invalid_request_error")
			return
		}
		RespondAnthropicError(c, http.StatusBadRequest, "invalid request body", "invalid_request_error")
		return
	}
	if req.Model == "" || len(req.Messages) == 0 {
		RespondAnthropicError(c, http.StatusBadRequest, "model and messages are required", "invalid_request_error")
		return
	}
	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondAnthropicError(c, http.StatusForbidden, "model is not permitted for this API key", "permission_error")
		return
	}
	// A14: a model that is not routed at all must be a 404, not a local estimate.
	// Without this, count_tokens answered 200 with a plausible token count for
	// models the gateway does not serve, so SDK clients kept going and only the
	// real inference call failed.
	if len(h.dispatcher.ListChannelsForModel(req.Model, "")) == 0 {
		RespondAnthropicError(c, http.StatusNotFound,
			fmt.Sprintf("model '%s' does not exist or is not available", req.Model), "not_found_error")
		return
	}
	channels := h.dispatcher.ResolveCandidateChannels(c.Request.Context(), req.Model, "anthropic_messages")
	var lastErr error
	// B-07: count_tokens dials the channel directly instead of going through
	// Dispatch*, so its circuit-breaker bookkeeping is bridged explicitly. The
	// classification matches the dispatch paths: transport/5xx/401/403/408 errors
	// are channel failures, while a client cancellation or a non-health outcome
	// only hands the half-open canary slot back. err == nil heals the channel.
	recordOutcome := func(channelName string, err error) {
		switch {
		case err == nil:
			h.dispatcher.NoteChannelSuccess(channelName)
		case c.Request.Context().Err() != nil || errors.Is(err, context.Canceled):
			h.dispatcher.ReleaseHalfOpenProbe(channelName)
		case provider.IsChannelFailure(err):
			h.dispatcher.NoteChannelFailure(channelName)
		default:
			h.dispatcher.ReleaseHalfOpenProbe(channelName)
		}
	}
	for _, channel := range channels {
		// count_tokens is an Anthropic-wire endpoint: only send it to channels
		// whose upstream actually speaks that protocol (type=anthropic or an
		// explicit anthropic_native marker). OpenAI/Gemini channels fall through
		// to the local estimate below instead of receiving Anthropic JSON.
		if !router.IsAnthropicWireChannel(channel) {
			continue
		}
		// B-07: this loop dials the channel directly, outside Dispatch*, so it used
		// to send traffic to a tripped circuit breaker and then report nothing
		// back. Admission now honours the breaker (including a half-open canary
		// slot) and the outcome is recorded through the accessors below.
		if !h.dispatcher.ChannelCanExecute(channel.Name) {
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
			// Local failure, not a channel-health signal: hand the canary back.
			recordOutcome(channel.Name, err)
			continue
		}
		// Shared URL builder: keeps /v1, /v1/messages and bare-base channels in
		// sync with the provider package instead of duplicating the suffix table.
		url := provider.BuildAnthropicURL(channel.BaseURL, "count_tokens")
		httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			recordOutcome(channel.Name, err)
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if channel.APIKey != "" && channel.APIKey != "none" {
			httpReq.Header.Set("x-api-key", channel.APIKey)
			httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		}
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		if beta := c.GetHeader("anthropic-beta"); beta != "" {
			httpReq.Header.Set("anthropic-beta", beta)
		}
		resp, err := provider.SharedDefaultHTTPClient.Do(httpReq)
		if err != nil {
			lastErr = err
			recordOutcome(channel.Name, err)
			continue
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			recordOutcome(channel.Name, readErr)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("channel %s returned HTTP %d: %s", channel.Name, resp.StatusCode, responseBody)
			// Classify with the status-carrying error so 429 stays backpressure and
			// 5xx trips the breaker, exactly like the dispatch paths (B-03/B-10).
			recordOutcome(channel.Name, provider.NewUpstreamError(channel.Name, resp.StatusCode, responseBody))
			continue
		}
		var result struct {
			InputTokens int `json:"input_tokens"`
		}
		if err := json.Unmarshal(responseBody, &result); err != nil {
			lastErr = err
			recordOutcome(channel.Name, err)
			continue
		}
		// The upstream answered a usable count: this channel is healthy.
		recordOutcome(channel.Name, nil)
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
	RespondAnthropicError(c, http.StatusBadGateway, sanitizeErrorMessage(lastErr), "api_error")
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
	tokens := model.EstimateTokensFromChars(chars)
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}