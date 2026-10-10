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

// ChatCompleteStream executes a streaming Anthropic request and translates SSE chunks to OpenAI format.
func (p *AnthropicProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	anthropicReq := convertOpenAIToAnthropic(req, channel)
	anthropicReq.Stream = true

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
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-api-key", channel.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect to anthropic %s stream error: %w", channel.Name, err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upstream anthropic %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	eventChan := make(chan *model.StreamEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		reader := bufio.NewReader(resp.Body)
		messageID := fmt.Sprintf("chatcmpl-claude-%d", time.Now().Unix())
		created := time.Now().Unix()

		var currentEvent string
		inputTokens := 0
		outputTokens := 0

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

			if strings.HasPrefix(lineStr, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(lineStr, "event:"))
				continue
			}

			if strings.HasPrefix(lineStr, "data:") {
				dataStr := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
				var eventMap map[string]any
				if err := json.Unmarshal([]byte(dataStr), &eventMap); err != nil {
					continue
				}

				switch currentEvent {
				case "message_start":
					if msgObj, ok := eventMap["message"].(map[string]any); ok {
						if id, ok := msgObj["id"].(string); ok && id != "" {
							messageID = id
						}
						if u, ok := msgObj["usage"].(map[string]any); ok {
							if in, ok := u["input_tokens"].(float64); ok {
								inputTokens = int(in)
							}
						}
					}
					// Emit initial chunk with role assistant
					chunk := &model.ChatCompletionChunk{
						ID:      messageID,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   req.Model,
						Choices: []model.ChunkChoice{
							{
								Index: 0,
								Delta: model.ChunkDelta{
									Role: "assistant",
								},
							},
						},
					}
					if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
						return
					}

				case "content_block_start":
					blockIdx := 0
					if bi, ok := eventMap["index"].(float64); ok {
						blockIdx = int(bi)
					}
					if cb, ok := eventMap["content_block"].(map[string]any); ok {
						if cb["type"] == "tool_use" {
							toolID, _ := cb["id"].(string)
							toolName, _ := cb["name"].(string)
							chunk := &model.ChatCompletionChunk{
								ID:      messageID,
								Object:  "chat.completion.chunk",
								Created: created,
								Model:   req.Model,
								Choices: []model.ChunkChoice{
									{
										Index: 0,
										Delta: model.ChunkDelta{
											ToolCalls: []model.ToolCall{
												{
													Index: &blockIdx,
													ID:    toolID,
													Type:  "function",
													Function: model.FunctionCall{
														Name: toolName,
													},
												},
											},
										},
									},
								},
							}
							if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
								return
							}
						}
					}

				case "content_block_delta":
					blockIdx := 0
					if bi, ok := eventMap["index"].(float64); ok {
						blockIdx = int(bi)
					}
					if delta, ok := eventMap["delta"].(map[string]any); ok {
						if text, ok := delta["text"].(string); ok && text != "" {
							chunk := &model.ChatCompletionChunk{
								ID:      messageID,
								Object:  "chat.completion.chunk",
								Created: created,
								Model:   req.Model,
								Choices: []model.ChunkChoice{
									{
										Index: 0,
										Delta: model.ChunkDelta{
											Content: text,
										},
									},
								},
							}
							if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
								return
							}
						} else if partial, ok := delta["partial_json"].(string); ok {
							chunk := &model.ChatCompletionChunk{
								ID:      messageID,
								Object:  "chat.completion.chunk",
								Created: created,
								Model:   req.Model,
								Choices: []model.ChunkChoice{
									{
										Index: 0,
										Delta: model.ChunkDelta{
											ToolCalls: []model.ToolCall{
												{
													Index: &blockIdx,
													Type:  "function",
													Function: model.FunctionCall{
														Arguments: partial,
													},
												},
											},
										},
									},
								},
							}
							if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
								return
							}
						}
					}

				case "message_delta":
					if u, ok := eventMap["usage"].(map[string]any); ok {
						if out, ok := u["output_tokens"].(float64); ok {
							outputTokens = int(out)
						}
					}
					stopReason := "stop"
					if delta, ok := eventMap["delta"].(map[string]any); ok {
						if r, ok := delta["stop_reason"].(string); ok {
							if r == "max_tokens" {
								stopReason = "length"
							} else if r == "tool_use" {
								stopReason = "tool_calls"
							}
						}
					}

					chunk := &model.ChatCompletionChunk{
						ID:      messageID,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   req.Model,
						Choices: []model.ChunkChoice{
							{
								Index:        0,
								Delta:        model.ChunkDelta{},
								FinishReason: &stopReason,
							},
						},
						Usage: &model.Usage{
							PromptTokens:     inputTokens,
							CompletionTokens: outputTokens,
							TotalTokens:      inputTokens + outputTokens,
						},
					}
					if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
						return
					}

				case "message_stop":
					sendEvent(&model.StreamEvent{IsDone: true})
					return
				}
			}
		}
	}()

	return eventChan, nil
}
