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

// AnthropicTool represents a tool definition in Claude Messages API.
type AnthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// AnthropicRequest represents the Anthropic Messages API payload.
type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // can be string or []any of content blocks
}

type AnthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []AnthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      AnthropicUsage          `json:"usage"`
}

// convertOpenAIToAnthropic converts OpenAI ChatCompletionRequest to Anthropic format.
func convertOpenAIToAnthropic(req *model.ChatCompletionRequest, channel *model.ChannelConfig) *AnthropicRequest {
	var systemParts []string
	var anthropicMsgs []AnthropicMessage

	for _, msg := range req.Messages {
		if strings.ToLower(msg.Role) == "system" {
			systemParts = append(systemParts, msg.GetContentString())
			continue
		}

		if strings.ToLower(msg.Role) == "tool" {
			// Convert tool response to user role with tool_result block
			toolResultBlock := map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.GetContentString(),
			}
			if len(anthropicMsgs) > 0 && anthropicMsgs[len(anthropicMsgs)-1].Role == "user" {
				if blocks, ok := anthropicMsgs[len(anthropicMsgs)-1].Content.([]any); ok {
					anthropicMsgs[len(anthropicMsgs)-1].Content = append(blocks, toolResultBlock)
				} else {
					anthropicMsgs[len(anthropicMsgs)-1].Content = []any{
						map[string]any{"type": "text", "text": fmt.Sprint(anthropicMsgs[len(anthropicMsgs)-1].Content)},
						toolResultBlock,
					}
				}
			} else {
				anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
					Role:    "user",
					Content: []any{toolResultBlock},
				})
			}
			continue
		}

		if strings.ToLower(msg.Role) == "assistant" && len(msg.ToolCalls) > 0 {
			var blocks []any
			contentStr := msg.GetContentString()
			if contentStr != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": contentStr,
				})
			}
			for _, tc := range msg.ToolCalls {
				var parsedInput any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsedInput); err != nil {
					parsedInput = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": parsedInput,
				})
			}
			anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
			continue
		}

		parts := model.ParseMessageContent(msg.Content)
		hasImage := false
		for _, p := range parts {
			if p.Type == model.ContentPartImageURL {
				hasImage = true
				break
			}
		}

		if !hasImage {
			anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
				Role:    msg.Role,
				Content: msg.GetContentString(),
			})
		} else {
			var blocks []any
			for _, p := range parts {
				switch p.Type {
				case model.ContentPartText:
					if p.Text != "" {
						blocks = append(blocks, map[string]any{
							"type": "text",
							"text": p.Text,
						})
					}
				case model.ContentPartImageURL:
					if p.ImageURL != nil && p.ImageURL.URL != "" {
						mime, b64 := model.ParseDataURI(p.ImageURL.URL)
						if mime == "" {
							mime = "image/jpeg"
						}
						blocks = append(blocks, map[string]any{
							"type": "image",
							"source": map[string]any{
								"type":       "base64",
								"media_type": mime,
								"data":       b64,
							},
						})
					}
				}
			}
			anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
				Role:    msg.Role,
				Content: blocks,
			})
		}
	}

	var tools []AnthropicTool
	for _, t := range req.Tools {
		fnName, _ := t.Function["name"].(string)
		fnDesc, _ := t.Function["description"].(string)
		fnParams := t.Function["parameters"]
		if fnParams == nil {
			fnParams = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, AnthropicTool{
			Name:        fnName,
			Description: fnDesc,
			InputSchema: fnParams,
		})
	}

	maxTokens := 4096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	return &AnthropicRequest{
		Model:       channel.GetUpstreamModel(req.Model),
		Messages:    anthropicMsgs,
		System:      strings.Join(systemParts, "\n\n"),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       tools,
	}
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
					eventChan <- &model.StreamEvent{Chunk: chunk}

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
							eventChan <- &model.StreamEvent{Chunk: chunk}
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
							eventChan <- &model.StreamEvent{Chunk: chunk}
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
							eventChan <- &model.StreamEvent{Chunk: chunk}
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
					eventChan <- &model.StreamEvent{Chunk: chunk}

				case "message_stop":
					eventChan <- &model.StreamEvent{IsDone: true}
					return
				}
			}
		}
	}()

	return eventChan, nil
}
