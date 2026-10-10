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

// ChatComplete executes a non-streaming Anthropic request and converts the response to OpenAI format.
func (p *AnthropicProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	anthropicReq := convertOpenAIToAnthropic(req, channel)
	anthropicReq.Stream = false

	payloadBytes, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request error: %w", err)
	}

	baseURL := strings.TrimRight(channel.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1/messages") {
		baseURL += "/v1/messages"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create anthropic http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", channel.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

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
		return nil, fmt.Errorf("upstream anthropic %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(bodyBytes, &anthropicResp); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response error: %w", err)
	}

	var fullContent strings.Builder
	var toolCalls []model.ToolCall
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			fullContent.WriteString(block.Text)
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
					Role:      "assistant",
					Content:   fullContent.String(),
					ToolCalls: toolCalls,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: &model.Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}

	return openAIResp, nil
}
