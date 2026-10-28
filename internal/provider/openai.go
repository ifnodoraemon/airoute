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
	"regexp"
	"strconv"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

var (
	// Regex for vLLM / GPUStack style context limit errors:
	// "maximum context length of 8192 tokens. You requested a total of 8618 tokens: 426 tokens from the input messages and 8192 tokens for the completion."
	contextLimitRegex1 = regexp.MustCompile(`(?i)maximum context length of (\d+) tokens.*?(\d+) tokens from the input messages`)
	// Regex for OpenAI style context limit errors:
	// "maximum context length is 8192 tokens. However, your messages resulted in 426 tokens and you requested 8192 tokens for the completion"
	contextLimitRegex2 = regexp.MustCompile(`(?i)maximum context length is (\d+) tokens.*?resulted in (\d+) tokens`)
)

func extractClampedMaxTokens(bodyBytes []byte) (int, bool) {
	str := string(bodyBytes)
	for _, re := range []*regexp.Regexp{contextLimitRegex1, contextLimitRegex2} {
		matches := re.FindStringSubmatch(str)
		if len(matches) >= 3 {
			maxCtx, err1 := strconv.Atoi(matches[1])
			inputTokens, err2 := strconv.Atoi(matches[2])
			if err1 == nil && err2 == nil && maxCtx > inputTokens {
				safeMax := maxCtx - inputTokens - 1
				if safeMax > 16 {
					return safeMax, true
				}
			}
		}
	}
	return 0, false
}

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

// buildURL joins the configured API root with /chat/completions using the shared
// APIURL semantics (bare origin gets /v1, a /v1 root is not doubled, custom
// prefixes and trailing slashes are preserved).
func buildURL(baseURL string) string {
	return APIURL(baseURL, "/v1/chat/completions")
}

// ChatComplete executes a standard non-streaming chat request.
func (p *OpenAIProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)

	// Automatic full-duplex protocol adaptation for text completion upstreams (/v1/completions)
	if channel.OnlySupportsProtocol("openai_text") || (channel.SupportsProtocol("openai_text") && !channel.SupportsProtocol("openai_chat")) {
		return p.chatCompleteText(ctx, req, channel, targetModel)
	}

	clonedReq := *req
	clonedReq.Model = targetModel
	clonedReq.Stream = false
	clonedReq.Messages = stripReasoningContent(clonedReq.Messages)

	for attempt := 0; ; attempt++ {
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

		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response body error: %w", readErr)
		}

		if resp.StatusCode != http.StatusOK {
			// Auto-heal context length overflow at most once. Recurring on every
			// 400 would let a misbehaving upstream drive an unbounded retry chain
			// (each pass shrinking max_tokens by one).
			if resp.StatusCode == http.StatusBadRequest && attempt == 0 && clonedReq.MaxTokens != nil {
				if safeMax, ok := extractClampedMaxTokens(bodyBytes); ok && safeMax < *clonedReq.MaxTokens {
					slog.Warn("auto-clamping max_tokens to fit upstream context limit",
						"upstream", channel.Name,
						"model", req.Model,
						"original_max_tokens", *clonedReq.MaxTokens,
						"clamped_max_tokens", safeMax,
					)
					clonedReq.MaxTokens = &safeMax
					continue
				}
			}
			return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
		}

		var chatResp model.ChatCompletionResponse
		if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
			return nil, fmt.Errorf("unmarshal chat response error: %w", err)
		}
		chatResp.Model = req.Model

		SeparateReasoningInResponse(&chatResp)

		return &chatResp, nil
	}
}

// stripReasoningContent returns a copy of messages with historical reasoning
// traces removed. reasoning_content is a DeepSeek-specific extension; strict
// OpenAI-compatible endpoints (Azure OpenAI in particular) reject unknown message
// fields with HTTP 400.
//
// It must copy: the outbound body is a per-channel adaptation, while the request
// object is shared by every later channel attempt, the cross-model fallback, the
// prompt-size estimators and the audit/trace hooks. Clearing the caller's slice
// in place let one channel's sanitising leak into all of them
// (BUG-REQ-MUTATION).
func stripReasoningContent(messages []model.ChatMessage) []model.ChatMessage {
	if len(messages) == 0 {
		return messages
	}
	out := make([]model.ChatMessage, len(messages))
	copy(out, messages)
	for i := range out {
		out[i].ReasoningContent = ""
	}
	return out
}

// ChatCompleteStream executes a streaming chat request.
func (p *OpenAIProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	return p.chatCompleteStream(ctx, req, channel, true)
}

// chatCompleteStream is ChatCompleteStream with an explicit clamp-retry budget so
// the context-overflow auto-heal can never recurse without bound.
func (p *OpenAIProvider) chatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig, allowClamp bool) (<-chan *model.StreamEvent, error) {
	targetModel := channel.GetUpstreamModel(req.Model)

	// Automatic full-duplex streaming protocol adaptation for text completion upstreams (/v1/completions)
	if channel.OnlySupportsProtocol("openai_text") || (channel.SupportsProtocol("openai_text") && !channel.SupportsProtocol("openai_chat")) {
		return p.chatCompleteTextStream(ctx, req, channel, targetModel)
	}

	clonedReq := *req
	clonedReq.Model = targetModel
	clonedReq.Stream = true
	clonedReq.StreamOptions = &model.StreamOptions{IncludeUsage: true}
	clonedReq.Messages = stripReasoningContent(clonedReq.Messages)

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
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
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

			line, err := reader.ReadBytes('\n')
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
