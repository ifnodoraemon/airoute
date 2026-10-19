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

func (p *OpenAIProvider) chatCompleteText(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig, targetModel string) (*model.ChatCompletionResponse, error) {
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
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
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
	SeparateReasoningInResponse(&chatResp)
	return &chatResp, nil
}

func (p *OpenAIProvider) chatCompleteTextStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig, targetModel string) (<-chan *model.StreamEvent, error) {
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
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
	}
	eventChan := make(chan *model.StreamEvent, 64)
	go func() {
		defer resp.Body.Close()
		defer close(eventChan)
		reader := bufio.NewReader(resp.Body)

		// send is context-aware: a bare channel send would block forever once
		// the consumer (client / pipe) goes away, leaking this goroutine and the
		// upstream response body for the lifetime of the process.
		send := func(ev *model.StreamEvent) bool {
			select {
			case eventChan <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					send(&model.StreamEvent{Err: err})
					return
				}
				send(&model.StreamEvent{IsDone: true})
				return
			}
			lineStr := strings.TrimSpace(string(line))
			if lineStr == "" || !strings.HasPrefix(lineStr, "data:") {
				continue
			}
			dataContent := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
			if dataContent == "[DONE]" {
				send(&model.StreamEvent{IsDone: true})
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
				if !send(&model.StreamEvent{Raw: line}) {
					return
				}
				continue
			}

			// Preserve every choice and its real index so `n > 1` keeps working
			// through the text-completion adaptation (previously only choices[0]
			// survived and was renumbered to 0).
			choices := make([]model.ChunkChoice, 0, len(textChunk.Choices))
			for _, ch := range textChunk.Choices {
				choices = append(choices, model.ChunkChoice{
					Index: ch.Index,
					Delta: model.ChunkDelta{
						Role:    "assistant",
						Content: ch.Text,
					},
					FinishReason: ch.FinishReason,
				})
			}
			if len(choices) == 0 {
				// Usage-only trailer frame.
				choices = []model.ChunkChoice{}
			}
			chatChunk := model.ChatCompletionChunk{
				ID:      textChunk.ID,
				Object:  "chat.completion.chunk",
				Created: textChunk.Created,
				Model:   req.Model,
				Choices: choices,
				Usage:   textChunk.Usage,
			}
			if !send(&model.StreamEvent{Chunk: &chatChunk}) {
				return
			}
		}
	}()
	return eventChan, nil
}
