package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func (h *MCPHandler) executeChatTool(ctx context.Context, args map[string]interface{}) (string, bool, bool) {
	name, _ := args["model"].(string)
	message, _ := args["message"].(string)
	session, _ := args["session_id"].(string)
	if name == "" || message == "" {
		return "Error: 'model' and 'message' are required arguments", true, true
	}
	caller, failure := h.chatToolCaller(ctx, args)
	if failure != "" {
		return failure, true, true
	}
	quota, failure := h.chatToolQuota(ctx, caller, name)
	if failure != "" {
		return failure, true, true
	}
	defer quota.Close()
	group := "default"
	if caller != nil && caller.Config != nil && caller.Config.GroupName != "" {
		group = caller.Config.GroupName
	}
	if err := validateMCPModelCurrency(name, group); err != nil {
		return "Error: " + err.Error(), true, true
	}
	req := &model.ChatCompletionRequest{Model: name, Messages: []model.ChatMessage{{Role: "user", Content: message}}}
	if raw, exists := args["max_tokens"]; exists {
		data, _ := json.Marshal(raw)
		var cap int
		if err := json.Unmarshal(data, &cap); err != nil || cap <= 0 {
			return "Error: max_tokens must be a positive integer", true, true
		}
		req.MaxTokens = &cap
	}
	if quota != nil {
		if billing.GlobalEngine == nil {
			return "Error: Billing prices are unavailable", true, true
		}
		if err := boundPrepaidCompletion(req); err != nil {
			return "Error: Invalid output token limit", true, true
		}
		if err := quota.Resize(ctx, estimateQuotaCost(name, group, req.EstimatedPromptTokens(), *req.MaxTokens, 1)); err != nil {
			return "Error: " + middleware.QuotaErrorDenial(err).Message, true, true
		}
	}
	if session != "" {
		identity := "anon"
		if caller != nil && caller.Record != nil {
			identity = sessionIdentityFor(caller.Record.TenantID, caller.Record.UserID, caller.Key)
		}
		ctx = context.WithValue(ctx, router.ContextKeySessionID, scopedSessionID(identity, session))
	}
	return h.dispatchChatTool(ctx, req, caller, quota, session)
}

func (h *MCPHandler) chatToolCaller(ctx context.Context, args map[string]interface{}) (*mcpCaller, string) {
	key, _ := ctx.Value(contextKeyCallerKey).(string)
	digest := key != ""
	if key == "" {
		key, _ = args["api_key"].(string)
	}
	caller := callerFromContext(ctx)
	if caller != nil && caller.Key == key {
		return caller, ""
	}
	var access *mcpAccessError
	if digest {
		caller, access = h.resolveCallerDigest(key)
	} else {
		caller, access = h.resolveCaller(key)
	}
	if access == nil {
		return caller, ""
	}
	if access.StatusCode == http.StatusUnauthorized {
		if strings.TrimSpace(key) == "" {
			return nil, "Error: Authentication required for airoute_chat. Please provide a valid API key via Authorization header, ?apiKey= query param, or 'api_key' argument."
		}
		return nil, "Error: Invalid or disabled API key provided"
	}
	return nil, "Error: " + access.Message
}

func (h *MCPHandler) chatToolQuota(ctx context.Context, caller *mcpCaller, name string) (*middleware.RequestQuota, string) {
	if caller == nil || caller.Record == nil {
		return nil, ""
	}
	if denial := middleware.AuthorizeKey(caller.Config); denial != nil {
		return nil, "Error: " + denial.Message
	}
	if !middleware.ModelAllowed(caller.Record.AllowedModels, name) {
		return nil, fmt.Sprintf("Error: Model %q is not permitted for this API key", name)
	}
	quota, err := middleware.AcquireRequestQuota(ctx, h.repo, caller.Key)
	if err != nil {
		return nil, "Error: " + middleware.QuotaErrorDenial(err).Message
	}
	return quota, ""
}

func (h *MCPHandler) dispatchChatTool(ctx context.Context, req *model.ChatCompletionRequest, caller *mcpCaller, quota *middleware.RequestQuota, session string) (string, bool, bool) {
	if h.dispatcher == nil {
		return "No response choices returned from upstream model", true, true
	}
	start := time.Now()
	response, err := h.dispatcher.Dispatch(ctx, req)
	params := mcpChatUsage(caller, req, response, err, time.Since(start))
	params.Quota, params.SessionID = quota, session
	RecordUsage(nil, params)
	if err != nil {
		return fmt.Sprintf("Upstream Dispatch Error: %v", err), true, true
	}
	if len(response.Choices) > 0 {
		return response.Choices[0].Message.GetContentString(), false, true
	}
	return "No response choices returned from upstream model", true, true
}

func mcpChatUsage(caller *mcpCaller, req *model.ChatCompletionRequest, response *model.ChatCompletionResponse, dispatchErr error, duration time.Duration) AuditRecordParams {
	identity := newMCPUsageIdentity(caller)
	params := AuditRecordParams{Model: req.Model, Duration: duration, StatusCode: http.StatusOK,
		APIKey: identity.apiKey, TenantID: identity.tenantID, KeyGroup: identity.keyGroup, TraceID: identity.traceID, KeyConfig: identity.keyConfig}
	if dispatchErr != nil {
		params.StatusCode, _, _, params.Channel = resolveUpstreamFailureStatus(dispatchErr)
		params.ErrorMessage = dispatchErr.Error()
		if params.StatusCode == statusClientClosed {
			if failure, ok := router.AsDispatchError(dispatchErr); ok && failure.PromptTokens > 0 {
				params.PromptTokens, params.TotalTokens, params.UsageSource = failure.PromptTokens, failure.PromptTokens, usageSourceEstimated
			}
		}
		return params
	}
	params.PromptTokens, params.CompletionTokens, params.CachedTokens, params.UsageSource = resolveChatUsage(req, response)
	params.TotalTokens, params.ChatID, params.Channel = params.PromptTokens+params.CompletionTokens, response.ID, response.Channel
	return params
}
