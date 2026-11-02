package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func nativeMatches(req *model.ChatCompletionRequest, channel *model.ChannelConfig, protocol string) bool {
	if req.Native == nil || req.Native.TranslateOnly || req.Native.Protocol != protocol {
		return false
	}
	// An explicit capability list is already evidence that an omitted protocol
	// is unavailable. An empty list is unknown, so try the native endpoint first.
	if len(channel.Protocols) == 0 {
		return true
	}
	for _, capability := range channel.Protocols {
		switch protocol {
		case model.NativeChat:
			if capability == "*" || capability == "chat" || capability == "openai_chat" {
				return true
			}
		case model.NativeResponses:
			if capability == "*" || capability == "response" || capability == "responses" || capability == "openai_response" || capability == "openai_responses" {
				return true
			}
		case model.NativeMessages:
			if channel.Type == model.ProviderAnthropic || capability == "*" || capability == "messages" || capability == "anthropic_messages" || capability == "anthropic_native" {
				return true
			}
		case model.NativeGemini:
			return model.IsGeminiProvider(channel.Type)
		}
	}
	return false
}

func translationError(req *model.ChatCompletionRequest, channel *model.ChannelConfig) error {
	if req.Native == nil || req.Native.TranslationError == nil {
		return nil
	}
	return &UnsupportedTranslationError{Reason: req.Native.TranslationError.Error()}
}

// UnsupportedTranslationError leaves other native candidates eligible while
// preventing an incapable channel from silently stripping requested features.
type UnsupportedTranslationError struct{ Reason string }

func (e *UnsupportedTranslationError) Error() string {
	return "protocol translation unsupported: " + e.Reason
}

func maybeNativeUnary(ctx context.Context, client *http.Client, req *model.ChatCompletionRequest, channel *model.ChannelConfig, protocols ...string) (*model.ChatCompletionResponse, bool, error) {
	for _, protocol := range protocols {
		if nativeMatches(req, channel, protocol) {
			response, done, err := nativeUnary(ctx, client, req, channel)
			if done {
				return response, true, err
			}
			break
		}
	}
	if err := translationError(req, channel); err != nil {
		return nil, true, err
	}
	return nil, false, nil
}

func maybeNativeStream(ctx context.Context, client *http.Client, req *model.ChatCompletionRequest, channel *model.ChannelConfig, protocols ...string) (<-chan *model.StreamEvent, bool, error) {
	for _, protocol := range protocols {
		if nativeMatches(req, channel, protocol) {
			stream, done, err := nativeStreaming(ctx, client, req, channel)
			if done {
				return stream, true, err
			}
			break
		}
	}
	if err := translationError(req, channel); err != nil {
		return nil, true, err
	}
	return nil, false, nil
}

// RewriteNativeModel changes only the model field and leaves every other JSON
// value as RawMessage, including fields unknown to this gateway.
func RewriteNativeModel(body []byte, name string) []byte {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	if _, exists := object["model"]; exists {
		object["model"], _ = json.Marshal(name)
	}
	if nested, ok := object["response"]; ok {
		object["response"] = RewriteNativeModel(nested, name)
	}
	if nested, ok := object["interaction"]; ok {
		object["interaction"] = RewriteNativeModel(nested, name)
	}
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

func nativeHTTPRequest(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*http.Request, error) {
	protocol := req.Native.Protocol
	body := req.Native.Body
	var target string
	switch protocol {
	case model.NativeChat:
		target = buildURL(channel.BaseURL)
	case model.NativeResponses:
		target = APIURL(channel.BaseURL, "/v1/responses")
	case model.NativeMessages:
		target = buildAnthropicURL(channel.BaseURL)
	case model.NativeGemini:
		target = buildGeminiURL(GeminiBaseURL(channel), channel.GetUpstreamModel(req.Model), req.Stream)
	default:
		return nil, fmt.Errorf("unsupported native protocol %q", protocol)
	}
	if protocol != model.NativeGemini {
		body = RewriteNativeModel(body, channel.GetUpstreamModel(req.Model))
	}
	if protocol == model.NativeChat && req.Stream {
		body = nativeStreamUsage(body)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.Stream {
		r.Header.Set("Accept", "text/event-stream")
	}
	for name, value := range req.Native.Headers {
		r.Header.Set(name, value)
	}
	if protocol == model.NativeGemini {
		if err := ConfigureGoogleRequest(ctx, r, channel); err != nil {
			return nil, err
		}
	} else {
		setNativeAuth(r, protocol, channel.APIKey)
	}
	if protocol == model.NativeMessages && channel.Type != model.ProviderAnthropic && channel.APIKey != "" && channel.APIKey != "none" {
		r.Header.Set("Authorization", "Bearer "+channel.APIKey)
	}
	return r, nil
}

func setNativeAuth(r *http.Request, protocol, key string) {
	if protocol == model.NativeGemini {
		SetGeminiHeaders(r, key)
		return
	}
	if protocol == model.NativeMessages && r.Header.Get("anthropic-version") == "" {
		r.Header.Set("anthropic-version", "2023-06-01")
	}
	if key == "" || key == "none" {
		return
	}
	switch protocol {
	case model.NativeMessages:
		r.Header.Set("x-api-key", key)
	default:
		r.Header.Set("Authorization", "Bearer "+key)
	}
}

func nativeStreamUsage(body []byte) []byte {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	var options map[string]json.RawMessage
	_ = json.Unmarshal(object["stream_options"], &options)
	if options == nil {
		options = make(map[string]json.RawMessage)
	}
	options["include_usage"] = json.RawMessage("true")
	object["stream_options"], _ = json.Marshal(options)
	out, _ := json.Marshal(object)
	return out
}

// Only an explicit unsupported endpoint permits a same-channel translation.
// Model, payload, authentication, quota and server errors retain their meaning.
func nativeEndpointUnsupported(protocol string, status int, body []byte) bool {
	if protocol != model.NativeResponses && protocol != model.NativeMessages {
		return false
	}
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return true
	}
	if status != http.StatusNotFound {
		return false
	}
	s := strings.ToLower(string(body))
	if strings.Contains(s, "model_not_found") || strings.Contains(s, "deploymentnotfound") || strings.Contains(s, "model not found") {
		return false
	}
	return strings.Contains(s, "404 page not found") || strings.Contains(s, "endpoint") || strings.Contains(s, "route") || strings.Contains(s, "unexpected path") || strings.Contains(s, `"not_found"`) || strings.Contains(s, `"not found"`)
}

func readNativeError(resp *http.Response, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (bool, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return true, err
	}
	if nativeEndpointUnsupported(req.Native.Protocol, resp.StatusCode, body) {
		return false, nil
	}
	return true, NewUpstreamError(channel.Name, resp.StatusCode, body, resp.Header)
}
