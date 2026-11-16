package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// OpenAI streaming chat completion, separated from the unary path in openai.go
// per the streaming/unary split rule.

func (p *OpenAIProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	if stream, done, err := maybeNativeStream(ctx, p.client, req, channel, model.NativeChat, model.NativeResponses, model.NativeMessages); done {
		return stream, err
	}
	return p.chatCompleteStream(ctx, req, channel, true)
}

// chatCompleteStream is ChatCompleteStream with an explicit clamp-retry budget so
// the context-overflow auto-heal can never recurse without bound.
func (p *OpenAIProvider) chatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig, allowClamp bool) (<-chan *model.StreamEvent, error) {
	targetModel := channel.GetUpstreamModel(req.Model)

	clonedReq := *req
	clonedReq.Model = targetModel
	clonedReq.Stream = true
	clonedReq.StreamOptions = &model.StreamOptions{IncludeUsage: true}
	if !keepThinkingHistory(req) {
		clonedReq.Messages = stripReasoningContent(clonedReq.Messages)
	}

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
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()

		// Auto-heal context length overflow at most once (see ChatComplete).
		if allowClamp && resp.StatusCode == http.StatusBadRequest && clonedReq.MaxTokens != nil {
			if safeMax, ok := extractClampedMaxTokens(bodyBytes); ok && safeMax < *clonedReq.MaxTokens {
				slog.Warn("auto-clamping max_tokens in stream to fit upstream context limit",
					"upstream", channel.Name,
					"model", req.Model,
					"original_max_tokens", *clonedReq.MaxTokens,
					"clamped_max_tokens", safeMax,
				)
				retryReq := *req
				retryReq.MaxTokens = &safeMax
				// allowClamp=false guarantees the retry cannot recurse further.
				return p.chatCompleteStream(ctx, &retryReq, channel, false)
			}
		}
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes, resp.Header)
	}

	eventChan := make(chan *model.StreamEvent, 64)
	reasoning := newReasoningStreamPipeline()

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		reader := bufio.NewReader(resp.Body)
		sendEvent := func(ev *model.StreamEvent) bool {
			select {
			case eventChan <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		sendChunk := func(chunk *model.ChatCompletionChunk) bool {
			return sendEvent(&model.StreamEvent{Chunk: chunk})
		}

		// flushReasoning emits any text still held back by a partial tag.
		flushReasoning := func() bool {
			return reasoning.Flush(sendChunk)
		}

		for {
			select {
			case <-ctx.Done():
				sendEvent(&model.StreamEvent{Err: ctx.Err()})
				return
			default:
			}

			line, err := readCappedLine(reader)
			if err != nil && !(err == io.EOF && len(line) > 0) {
				if err != io.EOF {
					sendEvent(&model.StreamEvent{Err: err})
					return
				}
				// Upstream closed without [DONE]: still flush buffered tag text
				// so a trailing partial  thinking marker is not swallowed.
				flushReasoning()
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
				if !flushReasoning() {
					return
				}
				sendEvent(&model.StreamEvent{IsDone: true})
				return
			}

			var chunk model.ChatCompletionChunk
			if err := json.Unmarshal([]byte(dataContent), &chunk); err != nil {
				// send raw event if parsing fails
				if !sendEvent(&model.StreamEvent{Raw: line}) {
					return
				}
				continue
			}

			// Intercept upstream error events emitted in data frames (e.g. rate limit or overload)
			if strings.Contains(dataContent, `"error"`) && len(chunk.Choices) == 0 {
				var errResp struct {
					Error struct {
						Message string `json:"message"`
						Type    string `json:"type"`
						Code    any    `json:"code"`
					} `json:"error"`
				}
				if json.Unmarshal([]byte(dataContent), &errResp) == nil && errResp.Error.Message != "" {
					// Carry the status so the router can classify a mid-stream
					// abort with the same rules as a unary failure (B-02).
					sendEvent(&model.StreamEvent{
						Err: NewUpstreamError(channel.Name, streamErrorStatus(jsonStatusCode(errResp.Error.Code), errResp.Error.Type), []byte(errResp.Error.Message)),
					})
					return
				}
			}

			chunk.Model = req.Model

			if !reasoning.Emit(&chunk, sendChunk) {
				return
			}
		}
	}()

	return eventChan, nil
}

func keepThinkingHistory(req *model.ChatCompletionRequest) bool {
	return len(req.Tools) > 0 && req.PreparedThinking != nil && (req.PreparedThinking.Profile.Wire == "deepseek" || req.PreparedThinking.Profile.Wire == "deepseek-template")
}