package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

const invalidProbeEffort = "__airoute_invalid_effort__"

type thinkingProbeResponse struct {
	status int
	body   []byte
}

func buildThinkingProbeRequest(ch *model.ChannelConfig, name string, profile model.ThinkingProfile, control model.ThinkingControl) (string, []byte, error) {
	limit := 128
	if profile.Wire == "claude-budget" && control.Mode == "on" {
		limit += profile.Budgets[profile.DefaultEffort]
	}
	raw, _ := json.Marshal(control)
	req := &model.ChatCompletionRequest{Model: name, MaxTokens: &limit, Messages: []model.ChatMessage{{Role: "user", Content: "Calculate 17 times 23. Reply with the number only."}}, Extra: model.ExtraFields{"thinking_control": raw}}
	channel := *ch
	channel.Type = model.ChatProviderType(ch, req)
	facts := &model.ThinkingConfig{CanDisable: &profile.Off, Efforts: profile.Efforts}
	if _, exists := model.FindThinkingProfile(profile.ID); exists {
		facts.Profile = profile.ID
	}
	channel.ModelMapping, channel.RouteRules = nil, nil
	channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{name: {Thinking: facts}}
	target := provider.APIURL(ch.BaseURL, "/v1/chat/completions")
	if model.IsGeminiProvider(ch.Type) {
		target = provider.GeminiActionURL(ch.BaseURL, name, "generateContent")
		body := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": req.Messages[0].GetContentString()}}}}, "generationConfig": map[string]any{"maxOutputTokens": limit}}
		req.Native = nativeThinkingProbe(model.NativeGemini, body)
	} else if model.ChatProviderType(ch, req) == model.ProviderAnthropic {
		target = provider.BuildAnthropicURL(ch.BaseURL, "")
		req.Native = nativeThinkingProbe(model.NativeMessages, map[string]any{"model": name, "messages": req.Messages, "max_tokens": limit})
	} else if thinkingProbeUsesResponses(ch.Protocols) {
		target = provider.APIURL(ch.BaseURL, "/v1/responses")
		req.Native = nativeThinkingProbe(model.NativeResponses, map[string]any{"model": name, "input": "Calculate 17 times 23. Reply with the number only.", "max_output_tokens": limit})
	}
	prepared, err := provider.PrepareThinkingRequest(req, &channel)
	if err != nil {
		return "", nil, err
	}
	if prepared.Native != nil {
		return target, prepared.Native.Body, nil
	}
	body, err := json.Marshal(prepared)
	return target, body, err
}

func thinkingProbeUsesResponses(protocols []string) bool {
	if slices.ContainsFunc(protocols, func(value string) bool { return slices.Contains([]string{"*", "chat", "openai_chat"}, value) }) {
		return false
	}
	return slices.ContainsFunc(protocols, func(value string) bool {
		return slices.Contains([]string{"response", "responses", "openai_response", "openai_responses"}, value)
	})
}

func nativeThinkingProbe(protocol string, body map[string]any) *model.NativeRequest {
	raw, _ := json.Marshal(body)
	return &model.NativeRequest{Protocol: protocol, Body: raw}
}

func invalidThinkingProbe(body []byte, profile model.ThinkingProfile, kind model.ProviderType) ([]byte, string) {
	var fields map[string]any
	_ = json.Unmarshal(body, &fields)
	parameter := "reasoning_effort"
	switch profile.Wire {
	case "claude-adaptive":
		parameter = "effort"
		fields["output_config"] = map[string]string{"effort": invalidProbeEffort}
	case "gemini-level", "gemini-summary":
		parameter = "thinkingLevel"
		if model.IsGeminiProvider(kind) {
			fields["generationConfig"].(map[string]any)["thinkingConfig"] = map[string]string{"thinkingLevel": invalidProbeEffort}
		} else {
			fields["extra_body"] = map[string]any{"google": map[string]any{"thinking_config": map[string]string{"thinking_level": invalidProbeEffort}}}
			parameter = "thinking_level"
		}
	case "deepseek-template":
		fields["chat_template_kwargs"].(map[string]any)["reasoning_effort"] = invalidProbeEffort
	default:
		if reasoning, ok := fields["reasoning"].(map[string]any); ok {
			reasoning["effort"] = invalidProbeEffort
			parameter = "effort"
		} else {
			fields[parameter] = invalidProbeEffort
		}
	}
	raw, _ := json.Marshal(fields)
	return raw, parameter
}

func (p *DownstreamProber) sendThinkingProbe(ctx context.Context, ch *model.ChannelConfig, target string, body []byte) (thinkingProbeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := validateProbeURL(target); err != nil {
		return thinkingProbeResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return thinkingProbeResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if model.IsGeminiProvider(ch.Type) {
		provider.SetGeminiHeaders(req, ch.APIKey)
	} else if strings.Contains(target, "/messages") {
		req.Header.Set("x-api-key", ch.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if ch.APIKey != "" && ch.APIKey != "none" {
		req.Header.Set("Authorization", "Bearer "+ch.APIKey)
	}
	client := *p.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return thinkingProbeResponse{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if len(data) > 1<<20 {
		return thinkingProbeResponse{}, fmt.Errorf("探测响应超过大小上限")
	}
	return thinkingProbeResponse{status: response.StatusCode, body: data}, err
}

func (response thinkingProbeResponse) rejects(parameter string) bool {
	if parameter == "" || (response.status != http.StatusBadRequest && response.status != http.StatusUnprocessableEntity) {
		return false
	}
	lower := strings.ToLower(string(response.body))
	return strings.Contains(lower, strings.ToLower(parameter)) && (strings.Contains(lower, "invalid") || strings.Contains(lower, "support") || strings.Contains(lower, "allowed") || strings.Contains(lower, "permitted") || strings.Contains(lower, "should be") || strings.Contains(lower, "must be one of"))
}

func (response thinkingProbeResponse) observation() (valid bool, tokens *int, thought bool) {
	if response.status != http.StatusOK {
		return false, nil, false
	}
	var body struct {
		Choices    []struct{ Message model.ChatMessage } `json:"choices"`
		Usage      *model.Usage                          `json:"usage"`
		Content    []struct{ Type string }               `json:"content"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					Thought bool `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Output        []struct{ Type string } `json:"output"`
		UsageMetadata struct {
			Thoughts *int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(response.body, &body) != nil || (len(body.Error) > 0 && string(body.Error) != "null") {
		return false, nil, false
	}
	for _, choice := range body.Choices {
		thought = thought || choice.Message.ReasoningContent != ""
	}
	for _, block := range body.Content {
		thought = thought || block.Type == "thinking" || block.Type == "redacted_thinking"
	}
	for _, candidate := range body.Candidates {
		for _, part := range candidate.Content.Parts {
			thought = thought || part.Thought
		}
	}
	for _, item := range body.Output {
		thought = thought || item.Type == "reasoning"
	}
	tokens = body.Usage.ReportedReasoningTokens()
	if tokens == nil && body.Usage != nil {
		var details struct {
			Reasoning *int `json:"reasoning_tokens"`
		}
		if json.Unmarshal(body.Usage.Extra["output_tokens_details"], &details) == nil {
			tokens = details.Reasoning
		}
	}
	if body.UsageMetadata.Thoughts != nil {
		tokens = body.UsageMetadata.Thoughts
	}
	return len(body.Choices)+len(body.Content)+len(body.Candidates)+len(body.Output) > 0, tokens, thought
}
