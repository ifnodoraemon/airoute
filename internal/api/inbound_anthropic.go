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
	res, err := adapter.AnthropicToCanonical(req)
	if err != nil {
		return &model.ChatCompletionRequest{Model: req.Model}
	}
	return res
}

// HandleAnthropicMessages handles POST /v1/messages for Anthropic SDK clients.
func (h *Handler) HandleAnthropicMessages(c *gin.Context) {
	CaptureRequestSnapshot(c)
	var req AnthropicInboundRequest
	native, err := bindNativeJSON(c, &req, model.NativeMessages)
	if err != nil {
		if isRequestBodyTooLarge(err) {
			RespondAnthropicError(c, http.StatusRequestEntityTooLarge,
				"Request body is too large for this endpoint. Reduce it or configure GATEWAY_MAX_BODY_BYTES.", "invalid_request_error")
			return
		}
		RespondAnthropicError(c, http.StatusBadRequest, fmt.Sprintf("Failed to parse Anthropic request: %v", err), "invalid_request_error")
		return
	}

	if req.Model == "" {
		RespondAnthropicError(c, http.StatusBadRequest, "model is required", "invalid_request_error")
		return
	}

	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondAnthropicError(c, http.StatusForbidden, fmt.Sprintf("Your API key is not permitted to access model '%s'", req.Model), "permission_error")
		return
	}

	// Validate Anthropic protocol and schema rules with optional key config
	valRes := validator.ValidateAnthropicRequest(&req, getAPIKeyConfig(c))
	if !valRes.Valid && valRes.Error != nil {
		rejectInvalidRequest(c, valRes.Error)
		return
	}
	if len(valRes.Warnings) > 0 {
		c.Header("X-Airoute-Warning", strings.Join(valRes.Warnings, "; "))
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	start := time.Now()
	canonicalReq := ConvertAnthropicToCanonical(&req)
	canonicalReq.Native = native
	canonicalReq.AnthropicBeta = c.GetHeader("anthropic-beta")

	sessionID := resolveSessionID(c, canonicalReq.Model, canonicalReq.Messages, "")
	// The provider call is detached from the downstream request so a customer
	// disconnect cannot cancel it before its usage report arrives.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionAffinityKey(c, sessionID))
	}

	if !req.Stream {
		h.handleAnthropicUnary(c, reqCtx, &req, canonicalReq, sessionID, start)
		return
	}

	h.handleAnthropicStream(c, reqCtx, &req, canonicalReq, sessionID, start)
}

func (h *Handler) handleAnthropicUnary(c *gin.Context, reqCtx context.Context, req *AnthropicInboundRequest, canonicalReq *model.ChatCompletionRequest, sessionID string, start time.Time) {
	if !topUpQuotaForRequest(c, canonicalReq) {
		return
	}
	resp, err := h.dispatcher.Dispatch(reqCtx, canonicalReq)
	if err != nil {
		statusCode, errType, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, sessionID, canonicalReq.Model, time.Since(start), statusCode, err)
		RespondAnthropicError(c, statusCode, sanitizeErrorMessage(err), errType)
		return
	}
	if len(resp.NativeResponse) > 0 {
		input, output, cached, source := resolveChatUsage(canonicalReq, resp)
		writes := 0
		if resp.Usage != nil {
			writes = resp.Usage.GetCacheCreationTokens()
		}
		RecordUsage(c, AuditRecordParams{SessionID: sessionID, ChatID: resp.ID, Channel: resp.Channel, Model: req.Model,
			PromptTokens: input, CompletionTokens: output, CachedTokens: cached, CacheCreationTokens: writes,
			Duration: time.Since(start), StatusCode: http.StatusOK, UsageSource: source})
		c.Data(http.StatusOK, "application/json", resp.NativeResponse)
		return
	}
	if len(resp.NativeAnthropicResponse) > 0 {
		var native map[string]any
		if err := json.Unmarshal(resp.NativeAnthropicResponse, &native); err == nil {
			native["model"] = req.Model
			inputTokens, outputTokens, cachedTokens, usageSource := resolveChatUsage(canonicalReq, resp)
			// C-02: cache writes are priced above the plain input rate. The
			// streaming path already carried them separately, but this unary path
			// read only cache_read_input_tokens, so prompt-cache writes were billed
			// as ordinary prompt tokens (or not at all). The native payload is
			// authoritative, so take the count from the same field the stream
			// parser uses.
			cacheCreationTokens := 0
			if usage, ok := native["usage"].(map[string]any); ok {
				cacheCreationTokens = nativeCacheCreationTokens(usage)
			}
			// PromptTokens includes cache reads and writes; passing them separately
			// lets the engine bill each at its own rate instead of the full prompt
			// rate (this path used to charge ~90% too much for cached prompts).
			RecordUsage(c, AuditRecordParams{SessionID: sessionID, ChatID: resp.ID, Channel: resp.Channel, Model: req.Model, PromptTokens: inputTokens + cacheCreationTokens, CompletionTokens: outputTokens, CachedTokens: cachedTokens, CacheCreationTokens: cacheCreationTokens, Duration: time.Since(start), StatusCode: http.StatusOK, UsageSource: usageSource})
			c.JSON(http.StatusOK, native)
			return
		}
	}

	contentBlocks := make([]gin.H, 0)
	stopReason := "end_turn"

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		// Do NOT synthesise a {"type":"thinking"} block from ReasoningContent.
		// The Anthropic Messages API requires every thinking block to carry a
		// provider signature, and clients echo assistant content back on the next
		// turn; an unsigned block therefore makes the follow-up request fail with
		// "thinking.signature: Field required". Reasoning from adapted
		// (non-Anthropic) upstreams is surfaced to OpenAI-shaped clients through
		// reasoning_content instead, and native Anthropic thinking blocks are
		// passed through verbatim by the native path above.
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
		} else if choice.FinishReason != nil && *choice.FinishReason == "content_filter" {
			stopReason = "refusal"
		}
	}

	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, gin.H{
			"type": "text",
			"text": "",
		})
	}

	pTokens, outputTokens, cachedTokens, usageSource := resolveChatUsage(canonicalReq, resp)
	// Anthropic's input_tokens excludes prompt-cache reads; our canonical
	// PromptTokens includes them, so subtract before rendering the response.
	inputTokens := pTokens - cachedTokens
	if inputTokens < 0 {
		inputTokens = 0
	}

	usagePayload := gin.H{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	}
	if cachedTokens > 0 {
		usagePayload["cache_read_input_tokens"] = cachedTokens
	}

	msgID := resp.ID
	if !strings.HasPrefix(msgID, "msg_") {
		msgID = "msg_" + msgID
	}

	anthropicResp := gin.H{
		"id":          msgID,
		"type":        "message",
		"role":        "assistant",
		"model":       req.Model,
		"content":     contentBlocks,
		"stop_reason": stopReason,
		"usage":       usagePayload,
	}

	// C-02: when the canonical response carries the prompt-cache write count (an
	// Anthropic-shaped upstream translated into the canonical struct), pass it
	// through so the cache-write premium is billed instead of the plain input
	// rate. Upstreams that do not report it contribute zero.
	cacheCreationTokens := 0
	if resp.Usage != nil {
		cacheCreationTokens = resp.Usage.GetCacheCreationTokens()
	}
	RecordUsage(c, AuditRecordParams{
		SessionID:           sessionID,
		ChatID:              resp.ID,
		Channel:             resp.Channel,
		Model:               req.Model,
		PromptTokens:        inputTokens + cachedTokens + cacheCreationTokens,
		CompletionTokens:    outputTokens,
		CachedTokens:        cachedTokens,
		CacheCreationTokens: cacheCreationTokens,
		Duration:            time.Since(start),
		StatusCode:          http.StatusOK,
		UsageSource:         usageSource,
	})

	c.JSON(http.StatusOK, anthropicResp)
}
