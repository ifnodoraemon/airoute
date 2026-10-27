package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// GeminiProvider handles Google's specialized Gemini Developer API & Vertex AI protocols.
type GeminiProvider struct {
	client *http.Client
}

// NewGeminiProvider creates a Gemini provider instance.
func NewGeminiProvider(client *http.Client) *GeminiProvider {
	if client == nil {
		client = SharedDefaultHTTPClient
	}
	return &GeminiProvider{client: client}
}

func (p *GeminiProvider) Name() string {
	return "google-gemini"
}

func (p *GeminiProvider) Type() model.ProviderType {
	return model.ProviderGemini
}

// buildGeminiURL creates the Google endpoint URL using the shared API root
// semantics: a bare origin becomes /v1beta, a root that already carries /v1beta
// is not doubled, and custom prefixes are preserved.
func buildGeminiURL(baseURL, modelName string, stream bool) string {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	endpoint := "/v1beta/models/" + modelName + ":generateContent"
	if stream {
		return joinAPIURLVersioned(baseURL, GeminiAPIVersion, "/v1beta/models/"+modelName+":streamGenerateContent", url.Values{"alt": {"sse"}})
	}
	return joinAPIURLVersioned(baseURL, GeminiAPIVersion, endpoint, nil)
}

// ChatComplete executes a non-streaming Gemini call and converts to OpenAI format.
func (p *GeminiProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	geminiReq := convertOpenAIToGemini(req)
	payloadBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request error: %w", err)
	}

	targetModel := channel.GetUpstreamModel(req.Model)
	targetURL := buildGeminiURL(channel.BaseURL, targetModel, false)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", channel.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do gemini request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response body error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response error: %w", err)
	}
	// A 200 body can still carry an error object (overload/quota/model failure);
	// returning an empty completion for it hid the failure from the client, the
	// audit row and the circuit breaker.
	if geminiResp.Error != nil {
		return nil, NewUpstreamError(channel.Name, http.StatusBadGateway, bodyBytes)
	}

	replyText := ""
	finishReason := "stop"
	var toolCalls []model.ToolCall
	if len(geminiResp.Candidates) > 0 {
		cand := geminiResp.Candidates[0]
		var sb strings.Builder
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				sb.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				argsBytes, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, model.ToolCall{
					ID:   fmt.Sprintf("call_%d_%s", time.Now().UnixMicro(), part.FunctionCall.Name),
					Type: "function",
					Function: model.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsBytes),
					},
				})
			}
		}
		replyText = sb.String()
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		} else if cand.FinishReason == "MAX_TOKENS" {
			finishReason = "length"
		}
	}

	var usage *model.Usage
	if geminiResp.UsageMetadata != nil {
		usage = geminiUsageToModel(geminiResp.UsageMetadata)
	}

	return &model.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []model.ChatCompletionChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:      "assistant",
					Content:   replyText,
					ToolCalls: toolCalls,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: usage,
	}, nil
}
