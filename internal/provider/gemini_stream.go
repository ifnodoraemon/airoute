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

// ChatCompleteStream executes a streaming Gemini call and converts SSE events to OpenAI format.
func (p *GeminiProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	if stream, done, err := maybeNativeStream(ctx, p.client, req, channel, model.NativeGemini); done {
		return stream, err
	}
	var historyErr error
	req, historyErr = restoreGeminiHistory(ctx, req, channel)
	if historyErr != nil {
		return nil, historyErr
	}
	if err := validateGeminiChatOptions(req); err != nil {
		return nil, err
	}
	geminiReq := convertOpenAIToGemini(req)
	targetModel := channel.GetUpstreamModel(req.Model)
	if err := applyGeminiThinking(geminiReq.GenerationConfig, req, targetModel); err != nil {
		return nil, err
	}
	payloadBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request error: %w", err)
	}

	targetURL := buildGeminiURL(GeminiBaseURL(channel), targetModel, true)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini stream request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if err := ConfigureGoogleRequest(ctx, httpReq, channel); err != nil {
		return nil, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect to gemini stream error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes, resp.Header)
	}

	eventChan := make(chan *model.StreamEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		reader := newSSEDataReader(resp.Body)
		msgID := fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixMicro())
		created := time.Now().Unix()
		choices := &geminiStreamChoices{}
		history := make(map[int]*geminiStreamHistory)

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

			line, err := reader.next()
			// An upstream (or an intermediary proxy) can close the connection right
			// after a complete data frame that lacks its trailing newline. Dropping
			// that buffered line lost the terminal frame — and with it the
			// finishReason, which the router reads as "upstream truncated the
			// stream" and charges to the channel. Process it, exactly like the
			// OpenAI reader already do.
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

			dataContent := lineStr
			if dataContent == "[DONE]" {
				sendEvent(&model.StreamEvent{IsDone: true})
				return
			}

			var geminiChunk GeminiResponse
			if err := json.Unmarshal([]byte(dataContent), &geminiChunk); err != nil {
				sendEvent(&model.StreamEvent{Err: fmt.Errorf("invalid Gemini SSE event: %w", err)})
				return
			}
			// Gemini reports some failures (overload, quota, model errors) as an
			// error object inside a 200 SSE stream. Without this the frame became an
			// empty normal chunk and the client saw a truncated but "successful"
			// completion.
			if geminiChunk.Error != nil {
				// Carry the status so the router classifies a mid-stream abort
				// with the same rules as a unary failure (B-02). Gemini's embedded
				// code is the unary HTTP equivalent; the status string is the
				// fallback for proxies that omit it.
				status := streamErrorStatus(geminiChunk.Error.Code, geminiChunk.Error.Status)
				sendEvent(&model.StreamEvent{Err: NewUpstreamError(channel.Name, status, []byte(geminiChunk.Error.Message))})
				return
			}

			var usage *model.Usage
			if geminiChunk.UsageMetadata != nil {
				usage = geminiUsageToModel(geminiChunk.UsageMetadata)
			}

			converted := choices.convert(&geminiChunk)
			if err := preserveGeminiStream(ctx, channel, req.Model, &geminiChunk, converted, history); err != nil {
				sendEvent(&model.StreamEvent{Err: err})
				return
			}
			chunk := &model.ChatCompletionChunk{
				ID:      msgID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: converted,
				Usage:   usage,
			}

			if !sendEvent(&model.StreamEvent{Chunk: chunk}) {
				return
			}
		}
	}()

	return eventChan, nil
}
