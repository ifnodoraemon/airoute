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
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ChatCompleteStream executes a streaming Gemini call and converts SSE events to OpenAI format.
func (p *GeminiProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	geminiReq := convertOpenAIToGemini(req)
	payloadBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request error: %w", err)
	}

	targetModel := channel.GetUpstreamModel(req.Model)
	targetURL := buildGeminiURL(channel.BaseURL, targetModel, true, channel.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini stream request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-goog-api-key", channel.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect to gemini stream error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upstream gemini %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	eventChan := make(chan *model.StreamEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		reader := bufio.NewReader(resp.Body)
		msgID := fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixNano())
		created := time.Now().Unix()

		sendEvent := func(ev *model.StreamEvent) bool {
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
				sendEvent(&model.StreamEvent{Err: ctx.Err()})
				return
			default:
			}

			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					sendEvent(&model.StreamEvent{Err: err})
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
				sendEvent(&model.StreamEvent{IsDone: true})
				return
			}

			var geminiChunk GeminiResponse
			if err := json.Unmarshal([]byte(dataContent), &geminiChunk); err != nil {
				continue
			}

			textDelta := ""
			var toolCalls []model.ToolCall
			var finishReason *string
			if len(geminiChunk.Candidates) > 0 {
				cand := geminiChunk.Candidates[0]
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						textDelta += part.Text
					}
					if part.FunctionCall != nil {
						argsBytes, _ := json.Marshal(part.FunctionCall.Args)
						toolCalls = append(toolCalls, model.ToolCall{
							ID:   fmt.Sprintf("call_%d_%s", time.Now().UnixNano(), part.FunctionCall.Name),
							Type: "function",
							Function: model.FunctionCall{
								Name:      part.FunctionCall.Name,
								Arguments: string(argsBytes),
							},
						})
					}
				}
				if len(toolCalls) > 0 {
					r := "tool_calls"
					finishReason = &r
				} else if cand.FinishReason != "" {
					r := strings.ToLower(cand.FinishReason)
					if r == "max_tokens" {
						r = "length"
					} else {
						r = "stop"
					}
					finishReason = &r
				}
			}

			var usage *model.Usage
			if geminiChunk.UsageMetadata != nil {
				usage = &model.Usage{
					PromptTokens:     geminiChunk.UsageMetadata.PromptTokenCount,
					CompletionTokens: geminiChunk.UsageMetadata.CandidatesTokenCount,
					TotalTokens:      geminiChunk.UsageMetadata.TotalTokenCount,
				}
			}

			chunk := &model.ChatCompletionChunk{
				ID:      msgID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: []model.ChunkChoice{
					{
						Index: 0,
						Delta: model.ChunkDelta{
							Content:   textDelta,
							ToolCalls: toolCalls,
						},
						FinishReason: finishReason,
					},
				},
				Usage: usage,
			}

			if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
				return
			}
		}
	}()

	return eventChan, nil
}
