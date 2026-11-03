package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func nativeUnary(ctx context.Context, client *http.Client, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, bool, error) {
	r, err := nativeHTTPRequest(ctx, req, channel)
	if err != nil {
		return nil, true, err
	}
	resp, err := client.Do(r)
	if err != nil {
		return nil, true, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		done, err := readNativeError(resp, req, channel)
		return nil, done, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	if nativeReplyWrongProtocol(req.Native.Protocol, body) {
		return nil, false, nil
	}
	result, err := nativeReply(req.Native.Protocol, body)
	if err != nil {
		return nil, true, fmt.Errorf("read native %s response: %w", req.Native.Protocol, err)
	}
	result.NativeResponse = RewriteNativeModel(body, req.Model)
	result.NativeProtocol = req.Native.Protocol
	result.NativeIdentity = model.UpstreamIdentity(channel)
	result.Model = req.Model
	return result, true, nil
}

// Some compatible relays return Chat JSON for every unknown path with HTTP 200.
// That is evidence that the requested Responses/Messages endpoint is absent.
func nativeReplyWrongProtocol(protocol string, body []byte) bool {
	if protocol != model.NativeResponses && protocol != model.NativeMessages {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return false
	}
	return len(object["choices"]) > 0 && len(object["output"]) == 0 && len(object["content"]) == 0
}

func nativeReply(protocol string, body []byte) (*model.ChatCompletionResponse, error) {
	result := &model.ChatCompletionResponse{}
	if protocol == model.NativeChat {
		return result, json.Unmarshal(body, result)
	}
	var envelope struct {
		ID            string               `json:"id"`
		ResponseID    string               `json:"responseId"`
		Usage         json.RawMessage      `json:"usage"`
		UsageMetadata *GeminiUsageMetadata `json:"usageMetadata"`
		Error         json.RawMessage      `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return nil, nativeEventError("", envelope.Error)
	}
	result.ID = envelope.ID
	if result.ID == "" {
		result.ID = envelope.ResponseID
	}
	result.Usage = nativeUsage(protocol, envelope.Usage, envelope.UsageMetadata)
	result.NativeOutputChars = nativeUnaryOutputChars(protocol, body)
	return result, nil
}

func nativeUsage(protocol string, raw json.RawMessage, gemini *GeminiUsageMetadata) *model.Usage {
	switch protocol {
	case model.NativeGemini:
		return geminiUsageToModel(gemini)
	case model.NativeMessages:
		var usage AnthropicUsage
		if json.Unmarshal(raw, &usage) == nil {
			return anthropicUsageToModel(&usage)
		}
	case model.NativeResponses:
		var usage struct {
			Input        int `json:"input_tokens"`
			Output       int `json:"output_tokens"`
			Total        int `json:"total_tokens"`
			InputDetails struct {
				Cached int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputDetails json.RawMessage `json:"output_tokens_details"`
		}
		if json.Unmarshal(raw, &usage) == nil {
			return &model.Usage{PromptTokens: usage.Input, CompletionTokens: usage.Output,
				TotalTokens:         max(usage.Total, usage.Input+usage.Output),
				PromptTokensDetails: cacheTokenDetails(usage.InputDetails.Cached, 0),
				Extra:               model.ExtraFields{"completion_tokens_details": usage.OutputDetails}}
		}
	}
	return nil
}
