package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// AnthropicProvider implements Provider for Anthropic Messages API.
type AnthropicProvider struct {
	client *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(client *http.Client) *AnthropicProvider {
	if client == nil {
		client = SharedDefaultHTTPClient
	}
	return &AnthropicProvider{client: client}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) Type() model.ProviderType {
	return model.ProviderAnthropic
}

// BuildAnthropicURL joins a channel base URL with an Anthropic Messages endpoint.
// endpoint is "" for /messages, or "count_tokens" for /messages/count_tokens.
// Exported so the count_tokens handler shares one implementation instead of
// duplicating the suffix table.
func BuildAnthropicURL(baseURL, endpoint string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	var messagesURL string
	switch {
	case strings.HasSuffix(baseURL, "/v1/messages"), strings.HasSuffix(baseURL, "/messages"):
		messagesURL = baseURL
	case strings.HasSuffix(baseURL, "/v1"):
		messagesURL = baseURL + "/messages"
	default:
		messagesURL = baseURL + "/v1/messages"
	}
	if endpoint == "" {
		return messagesURL
	}
	return messagesURL + "/" + strings.TrimPrefix(endpoint, "/")
}

// buildAnthropicURL joins base URL with /messages or /v1/messages cleanly.
func buildAnthropicURL(baseURL string) string {
	return BuildAnthropicURL(baseURL, "")
}

// ChatComplete executes a non-streaming Anthropic request and converts the response to OpenAI format.
func (p *AnthropicProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	anthropicReq := convertOpenAIToAnthropic(req, channel)
	anthropicReq.Stream = false

	payloadBytes, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request error: %w", err)
	}

	baseURL := buildAnthropicURL(channel.BaseURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create anthropic http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("x-api-key", channel.APIKey)
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
	}
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if req.AnthropicBeta != "" {
		httpReq.Header.Set("anthropic-beta", req.AnthropicBeta)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do anthropic request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read anthropic body error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(bodyBytes, &anthropicResp); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response error: %w", err)
	}

	var fullContent strings.Builder
	var reasoningBuilder strings.Builder
	var toolCalls []model.ToolCall
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			fullContent.WriteString(block.Text)
		} else if block.Type == "thinking" {
			reasoningBuilder.WriteString(block.Thinking)
		} else if block.Type == "tool_use" {
			toolCalls = append(toolCalls, model.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: model.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 || anthropicResp.StopReason == "tool_use" {
		finishReason = "tool_calls"
	} else if anthropicResp.StopReason == "max_tokens" {
		finishReason = "length"
	}

	openAIResp := &model.ChatCompletionResponse{
		ID:      anthropicResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []model.ChatCompletionChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:             "assistant",
					Content:          fullContent.String(),
					ReasoningContent: reasoningBuilder.String(),
					ToolCalls:        toolCalls,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: anthropicUsageToModel(&anthropicResp.Usage),
	}
	if req.AnthropicOriginal != nil {
		openAIResp.NativeAnthropicResponse = bodyBytes
	}

	return openAIResp, nil
}
