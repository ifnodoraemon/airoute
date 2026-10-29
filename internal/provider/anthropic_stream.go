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

	baseURL := buildAnthropicURL(channel.BaseURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create anthropic http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
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
		return nil, fmt.Errorf("connect to anthropic %s stream error: %w", channel.Name, err)
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
		messageID := fmt.Sprintf("chatcmpl-claude-%d", time.Now().Unix())
		created := time.Now().Unix()

		var currentEvent string
		inputTokens := 0
		outputTokens := 0
		cacheReadTokens := 0
		cacheCreationTokens := 0

		// Anthropic numbers content blocks (text block 0, thinking block 1, tool_use
		// block 2 ...), while OpenAI numbers tool calls only and they must start at
		// 0. Emitting the block index as the tool-call index produced tool_calls[2]
		// with no 0/1 entries whenever the model wrote text (or thought) before
		// calling a tool, which clients that accumulate by index cannot merge.
		toolIndexByBlock := make(map[int]int)
		nextToolIndex := 0
		toolIndexForBlock := func(block int) int {
			if idx, ok := toolIndexByBlock[block]; ok {
				return idx
			}
			idx := nextToolIndex
			nextToolIndex++
			toolIndexByBlock[block] = idx
			return idx
		}

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
			// A complete data frame can arrive without its trailing newline when the
			// upstream (or a proxy) cuts the connection. Process the buffered line
			// instead of dropping it: it may be the message_delta carrying
			// output_tokens or the message_stop terminal frame.
			if err != nil && !(err == io.EOF && len(line) > 0) {
				if err != io.EOF {
					sendEvent(&model.StreamEvent{Err: err})
					return
				}
				sendEvent(&model.StreamEvent{IsDone: true})
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
				// Anthropic can abort a stream with an `event: error` frame at any
				// point. Handle it for BOTH paths: previously the adapted
				// (OpenAI-shaped) path fell through the switch and silently
				// dropped it, so the downstream client saw a truncated but
				// "successful" 200 stream.
				if currentEvent == "error" {
					// Carry the status so the router classifies a mid-stream abort
					// with the same rules as a unary failure (B-02).
					var errFrame struct {
						Error struct {
							Type    string `json:"type"`
							Message string `json:"message"`
						} `json:"error"`
					}
					status := 503
					message := dataStr
					if json.Unmarshal([]byte(dataStr), &errFrame) == nil {
						status = streamErrorStatus(0, errFrame.Error.Type)
						if errFrame.Error.Message != "" {
							message = errFrame.Error.Message
						}
					}
					sendEvent(&model.StreamEvent{Err: NewUpstreamError(channel.Name, status, []byte(message))})
					return
				}
				if req.AnthropicOriginal != nil {
					if !sendEvent(&model.StreamEvent{NativeAnthropicEvent: []byte(dataStr), NativeAnthropicType: currentEvent, IsDone: currentEvent == "message_stop"}) {
						return
					}
					if currentEvent == "message_stop" {
						return
					}
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
							if cr, ok := u["cache_read_input_tokens"].(float64); ok {
								cacheReadTokens = int(cr)
							}
							if cc, ok := u["cache_creation_input_tokens"].(float64); ok {
								cacheCreationTokens = int(cc)
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
							toolIndex := toolIndexForBlock(blockIdx)
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
													Index: &toolIndex,
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
						} else if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
							// Extended thinking (thinking_delta) used to fall through and
							// vanish on the adapted path; keep it as reasoning_content so
							// OpenAI-shaped clients see the trace the upstream billed for.
							chunk := &model.ChatCompletionChunk{
								ID:      messageID,
								Object:  "chat.completion.chunk",
								Created: created,
								Model:   req.Model,
								Choices: []model.ChunkChoice{
									{
										Index: 0,
										Delta: model.ChunkDelta{
											ReasoningContent: thinking,
										},
									},
								},
							}
							if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
								return
							}
						} else if partial, ok := delta["partial_json"].(string); ok {
							toolIndex := toolIndexForBlock(blockIdx)
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
													Index: &toolIndex,
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
						if in, ok := u["input_tokens"].(float64); ok {
							inputTokens = int(in)
						}
						if cached, ok := u["cache_read_input_tokens"].(float64); ok {
							cacheReadTokens = int(cached)
						}
						if created, ok := u["cache_creation_input_tokens"].(float64); ok {
							cacheCreationTokens = int(created)
						}
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
							PromptTokens:     inputTokens + cacheReadTokens + cacheCreationTokens,
							CompletionTokens: outputTokens,
							TotalTokens:      inputTokens + cacheReadTokens + cacheCreationTokens + outputTokens,
							// Both cache buckets must survive the streaming path:
							// billing prices cache writes at a premium and needs the
							// count to split promptTokens into its three buckets.
							// Reporting only the read count billed the write bucket
							// at the plain input rate (F-05).
							PromptTokensDetails: cacheTokenDetails(cacheReadTokens, cacheCreationTokens),
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
