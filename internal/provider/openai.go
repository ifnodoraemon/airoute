package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// OpenAIProvider handles all OpenAI-compatible API endpoints (OpenAI, DeepSeek, vLLM, SGLang, Ollama).
type OpenAIProvider struct {
	client *http.Client
}

// NewOpenAIProvider creates an OpenAI compatible provider instance.
func NewOpenAIProvider(client *http.Client) *OpenAIProvider {
	if client == nil {
		client = SharedDefaultHTTPClient
	}
	return &OpenAIProvider{client: client}
}

func (p *OpenAIProvider) Name() string {
	return "openai-compatible"
}

func (p *OpenAIProvider) Type() model.ProviderType {
	return model.ProviderOpenAI
}

// buildURL joins base URL with /chat/completions cleanly.
func buildURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

// buildCompletionURL joins base URL with /completions cleanly.
func buildCompletionURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/completions") && !strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return strings.TrimSuffix(baseURL, "/chat/completions") + "/completions"
	}
	return baseURL + "/completions"
}

func convertChatToPrompt(messages []model.ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		role := strings.ToLower(m.Role)
		content := m.GetContentString()
		switch role {
		case "system":
			sb.WriteString("System: " + content + "\n")
		case "user":
			sb.WriteString("User: " + content + "\n")
		case "assistant":
			sb.WriteString("Assistant: " + content + "\n")
		default:
			sb.WriteString(content + "\n")
		}
	}
	sb.WriteString("Assistant: ")
	return sb.String()
}

// ChatComplete executes a standard non-streaming chat request.
func (p *OpenAIProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)

	// Automatic full-duplex protocol adaptation for text completion upstreams (/v1/completions)
	if channel.OnlySupportsProtocol("openai_text") || (channel.SupportsProtocol("openai_text") && !channel.SupportsProtocol("openai_chat")) {
		prompt := convertChatToPrompt(req.Messages)
		textReq := map[string]any{
			"model":       targetModel,
			"prompt":      prompt,
			"stream":      false,
			"temperature": req.Temperature,
			"top_p":       req.TopP,
		}
		if req.MaxTokens != nil {
			textReq["max_tokens"] = *req.MaxTokens
		}
		payloadBytes, err := json.Marshal(textReq)
		if err != nil {
			return nil, fmt.Errorf("marshal text completion request error: %w", err)
		}
		targetURL := buildCompletionURL(channel.BaseURL)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, fmt.Errorf("create http request error: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if channel.APIKey != "" && channel.APIKey != "none" {
			httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		}
		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("do http request to %s error: %w", channel.Name, err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body error: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("upstream %s returned status %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
		}
		var textResp struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			Choices []struct {
				Text         string  `json:"text"`
				Index        int     `json:"index"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *model.Usage `json:"usage"`
		}
		if err := json.Unmarshal(bodyBytes, &textResp); err != nil {
			return nil, fmt.Errorf("unmarshal text completion response error: %w", err)
		}
		chatResp := model.ChatCompletionResponse{
			ID:      textResp.ID,
			Object:  "chat.completion",
			Created: textResp.Created,
			Model:   req.Model,
			Usage:   textResp.Usage,
		}
		for _, ch := range textResp.Choices {
			chatResp.Choices = append(chatResp.Choices, model.ChatCompletionChoice{
				Index: ch.Index,
				Message: model.ChatMessage{
					Role:    "assistant",
					Content: ch.Text,
				},
				FinishReason: ch.FinishReason,
			})
		}
		return &chatResp, nil
	}

	clonedReq := *req
	clonedReq.Model = targetModel
	clonedReq.Stream = false

	payloadBytes, err := json.Marshal(clonedReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request error: %w", err)
	}

	targetURL := buildURL(channel.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do http request to %s error: %w", channel.Name, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s returned status %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var chatResp model.ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal chat response error: %w", err)
	}
	chatResp.Model = req.Model

	return &chatResp, nil
}

// ChatCompleteStream executes a streaming chat request.
func (p *OpenAIProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	targetModel := channel.GetUpstreamModel(req.Model)

	// Automatic full-duplex streaming protocol adaptation for text completion upstreams (/v1/completions)
	if channel.OnlySupportsProtocol("openai_text") || (channel.SupportsProtocol("openai_text") && !channel.SupportsProtocol("openai_chat")) {
		prompt := convertChatToPrompt(req.Messages)
		textReq := map[string]any{
			"model":       targetModel,
			"prompt":      prompt,
			"stream":      true,
			"temperature": req.Temperature,
			"top_p":       req.TopP,
		}
		if req.MaxTokens != nil {
			textReq["max_tokens"] = *req.MaxTokens
		}
		payloadBytes, err := json.Marshal(textReq)
		if err != nil {
			return nil, fmt.Errorf("marshal text completion request error: %w", err)
		}
		targetURL := buildCompletionURL(channel.BaseURL)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, fmt.Errorf("create http request error: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if channel.APIKey != "" && channel.APIKey != "none" {
			httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		}
		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("connect to %s stream error: %w", channel.Name, err)
		}
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("upstream %s returned status %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
		}
		eventChan := make(chan *model.StreamEvent, 64)
		go func() {
			defer resp.Body.Close()
			defer close(eventChan)
			reader := bufio.NewReader(resp.Body)
			for {
				select {
				case <-ctx.Done():
					eventChan <- &model.StreamEvent{Err: ctx.Err()}
					return
				default:
				}
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err != io.EOF {
						eventChan <- &model.StreamEvent{Err: err}
					}
					return
				}
				lineStr := strings.TrimSpace(string(line))
				if lineStr == "" || !strings.HasPrefix(lineStr, "data:") {
					continue
				}
				dataContent := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
				if dataContent == "[DONE]" {
					eventChan <- &model.StreamEvent{IsDone: true}
					return
				}
				var textChunk struct {
					ID      string `json:"id"`
					Created int64  `json:"created"`
					Choices []struct {
						Text         string  `json:"text"`
						Index        int     `json:"index"`
						FinishReason *string `json:"finish_reason"`
					} `json:"choices"`
					Usage *model.Usage `json:"usage"`
				}
				if err := json.Unmarshal([]byte(dataContent), &textChunk); err != nil {
					eventChan <- &model.StreamEvent{Raw: line}
					continue
				}
				deltaText := ""
				var finishReason *string
				if len(textChunk.Choices) > 0 {
					deltaText = textChunk.Choices[0].Text
					finishReason = textChunk.Choices[0].FinishReason
				}
				chatChunk := model.ChatCompletionChunk{
					ID:      textChunk.ID,
					Object:  "chat.completion.chunk",
					Created: textChunk.Created,
					Model:   req.Model,
					Choices: []model.ChunkChoice{
						{
							Index: 0,
							Delta: model.ChunkDelta{
								Role:    "assistant",
								Content: deltaText,
							},
							FinishReason: finishReason,
						},
					},
					Usage: textChunk.Usage,
				}
				eventChan <- &model.StreamEvent{
					Chunk: &chatChunk,
					Raw:   line,
				}
			}
		}()
		return eventChan, nil
	}

	clonedReq := *req
	clonedReq.Model = targetModel
	clonedReq.Stream = true

	payloadBytes, err := json.Marshal(clonedReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request error: %w", err)
	}
	targetURL := buildURL(channel.BaseURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect to %s stream error: %w", channel.Name, err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upstream %s returned status %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	eventChan := make(chan *model.StreamEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		reader := bufio.NewReader(resp.Body)
		for {
			select {
			case <-ctx.Done():
				eventChan <- &model.StreamEvent{Err: ctx.Err()}
				return
			default:
			}

			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					eventChan <- &model.StreamEvent{Err: err}
				}
				return
			}

			lineStr := strings.TrimSpace(string(line))
			if lineStr == "" {
				continue
			}

			if !strings.HasPrefix(lineStr, "data:") {
				continue
			}

			dataContent := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
			if dataContent == "[DONE]" {
				eventChan <- &model.StreamEvent{IsDone: true}
				return
			}

			var chunk model.ChatCompletionChunk
			if err := json.Unmarshal([]byte(dataContent), &chunk); err != nil {
				// send raw event if parsing fails
				eventChan <- &model.StreamEvent{Raw: line}
				continue
			}
			chunk.Model = req.Model

			eventChan <- &model.StreamEvent{
				Chunk: &chunk,
				Raw:   line,
			}
		}
	}()

	return eventChan, nil
}

// Embed executes an embedding generation request against standard OpenAI-compatible endpoints.
func (p *OpenAIProvider) Embed(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)
	cloned := *req
	cloned.Model = targetModel

	payloadBytes, err := json.Marshal(cloned)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request error: %w", err)
	}

	targetURL := strings.TrimRight(channel.BaseURL, "/") + "/embeddings"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create embedding http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		httpReq.Header.Set("x-api-key", channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do embedding request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream embedding %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var embResp model.EmbeddingResponse
	if err := json.Unmarshal(bodyBytes, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response error: %w", err)
	}

	// Restore user's requested model in the response
	embResp.Model = req.Model
	return &embResp, nil
}

// Rerank executes a cross-encoder rerank request against standard rerank endpoints.
func (p *OpenAIProvider) Rerank(ctx context.Context, req *model.RerankRequest, channel *model.ChannelConfig) (*model.RerankResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)
	cloned := *req
	cloned.Model = targetModel

	payloadBytes, err := json.Marshal(cloned)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request error: %w", err)
	}

	baseURL := strings.TrimRight(channel.BaseURL, "/")
	var targetURL string
	if strings.HasSuffix(baseURL, "/v1") {
		targetURL = baseURL + "/rerank"
	} else {
		targetURL = baseURL + "/v1/rerank"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create rerank http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		httpReq.Header.Set("x-api-key", channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do rerank request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream rerank %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var rerankResp model.RerankResponse
	if err := json.Unmarshal(bodyBytes, &rerankResp); err != nil {
		return nil, fmt.Errorf("unmarshal rerank response error: %w", err)
	}

	// Restore user's requested model in the response
	rerankResp.Model = req.Model
	return &rerankResp, nil
}

