package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// handleResponsesStreaming handles streaming SSE event translation for the Responses API.
func (h *Handler) handleResponsesStreaming(
	c *gin.Context,
	reqCtx context.Context,
	req model.ResponseRequest,
	chatReq *model.ChatCompletionRequest,
	sessionID string,
	start time.Time,
) {
	// Track the in-flight stream for the dashboard's live telemetry gauges.
	telemetry.GlobalLiveMetrics.BeginStream()
	defer telemetry.GlobalLiveMetrics.EndStream()

	if chatReq.StreamOptions == nil {
		chatReq.StreamOptions = &model.StreamOptions{IncludeUsage: true}
	} else {
		chatReq.StreamOptions.IncludeUsage = true
	}
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, chatReq)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}

	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondOpenAIError(c, http.StatusInternalServerError, "Streaming unsupported by response writer", "server_error", "streaming_unsupported")
		return
	}

	respID := fmt.Sprintf("resp_%d", time.Now().UnixMicro())
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixMicro())
	w := c.Writer
	emitter := newResponsesStreamEmitter(w, flusher, respID, msgID, req.Model)

	// 1-2. response.created + response.in_progress
	emitter.created()
	// 3-4. response.output_item.added + response.content_part.added
	emitter.itemAdded()

	firstTokenRecorded := false
	var ttftDuration time.Duration
	totalPromptTokens := 0
	totalCompTokens := 0
	totalCachedTokens := 0
	respUpstreamChatID := ""
	respUpstreamChannel := ""
	var streamError error

	recordStreamEnd := func(statusCode int) (float64, float64, int) {
		// A client abort (499) still sent the prompt upstream; estimate it too.
		if statusCode == http.StatusOK || statusCode == statusClientClosed {
			canonicalEstimate := estimateCanonicalPromptTokens(chatReq)
			if totalPromptTokens == 0 {
				totalPromptTokens = streamFallbackPromptTokens(chatReq)
			} else {
				observePromptEstimate(chatReq.Model, canonicalEstimate, totalPromptTokens)
			}
			// Tool-call arguments are completion tokens too; count them in the
			// fallback estimate so a tool-only stream is not billed as prompt-only.
			if totalCompTokens == 0 && emitter.fullText.Len()+emitter.toolChars > 0 {
				totalCompTokens = estimatedStreamCompletionTokens(emitter.fullText.Len() + emitter.toolChars)
			}
		}
		return recordUsage(c, AuditRecordParams{
			ChatID:           respUpstreamChatID,
			Channel:          respUpstreamChannel,
			SessionID:        sessionID,
			Model:            req.Model,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			// Prompt-cache reads are billed at the cache rate; without this the
			// stream reported every prompt token at the full rate (and the
			// response.completed usage/detail and trailer all said 0 cached).
			CachedTokens: totalCachedTokens,
			Duration:     time.Since(start),
			TTFT:         ttftDuration,
			StatusCode:   statusCode,
			ErrorMessage: sanitizeErrorMessage(streamError),
			LiveTracked:  true, // stream deltas already fed the live token meter
		})
	}

	for event := range streamChan {
		if respUpstreamChannel == "" && event.Channel != "" {
			respUpstreamChannel = event.Channel
		}

		if event.Err != nil {
			streamError = event.Err
			emitter.failed(event.Err)
			// An upstream failure must NOT be followed by
			// response.output_text.done / response.completed: clients that key
			// off the last event would otherwise treat a failed stream as a
			// successful completion. Record it as a failure and stop here.
			recordStreamEnd(http.StatusBadGateway)
			flusher.Flush()
			return
		}

		if event.IsDone {
			break
		}

		if event.Chunk != nil {
			if event.Chunk.ID != "" && respUpstreamChatID == "" {
				respUpstreamChatID = event.Chunk.ID
			}
			if !firstTokenRecorded {
				ttftDuration = time.Since(start)
				telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
				firstTokenRecorded = true
			}

			if event.Chunk.Usage != nil {
				totalPromptTokens = event.Chunk.Usage.PromptTokens
				totalCompTokens = event.Chunk.Usage.CompletionTokens
				totalCachedTokens = event.Chunk.Usage.GetCachedTokens()
			}

			for _, choice := range event.Chunk.Choices {
				deltaText := choice.Delta.Content
				if deltaText != "" {
					emitter.fullText.WriteString(deltaText)
					// 实时吞吐：流式 delta 到达即计入内存 token 计数。
					telemetry.GlobalLiveMetrics.AddChars(len(deltaText))
					emitter.textDelta(deltaText)
				}
				for _, toolCall := range choice.Delta.ToolCalls {
					toolIndex := 0
					if toolCall.Index != nil {
						toolIndex = *toolCall.Index
					}
					emitter.toolCallDelta(toolIndex, toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)
					if toolCall.Function.Arguments != "" {
						telemetry.GlobalLiveMetrics.AddChars(len(toolCall.Function.Arguments))
					}
				}
			}
		}
	}

	cost, savedCost, cachedTokens := recordStreamEnd(http.StatusOK)

	// Persist before the terminal event: a client that issues a follow-up with
	// previous_response_id as soon as it sees response.completed must already
	// find the conversation stored. A failure here is logged, not fatal.
	persistResponseContext(c, respID, chatReq.Messages, emitter.persistedAssistantMessage())

	// 5-9. output_text.done / content_part.done / output_item.done / response.completed
	emitter.completed(gin.H{
		"input_tokens":          totalPromptTokens,
		"output_tokens":         totalCompTokens,
		"total_tokens":          totalPromptTokens + totalCompTokens,
		"input_tokens_details":  gin.H{"cached_tokens": cachedTokens},
		"output_tokens_details": gin.H{"reasoning_tokens": 0},
	})

	// Billing metadata as an SSE comment, after the terminal event so no client
	// parses it as part of the response payload.
	writeStreamUsageTrailer(w, flusher, cost, savedCost, cachedTokens)

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// persistedAssistantMessage renders the assistant turn of the completed stream
// so it can be stored as conversation context for a previous_response_id
// follow-up. Tool calls are reconstructed from the emitter's accumulated state.
func (e *responsesStreamEmitter) persistedAssistantMessage() *model.ChatMessage {
	message := &model.ChatMessage{Role: "assistant"}
	if text := e.fullText.String(); text != "" {
		message.Content = text
	}
	for _, toolIndex := range e.toolOrder {
		state := e.toolStates[toolIndex]
		if state == nil {
			continue
		}
		message.ToolCalls = append(message.ToolCalls, model.ToolCall{
			ID:       state.callID,
			Type:     "function",
			Function: model.FunctionCall{Name: state.name, Arguments: state.arguments.String()},
		})
	}
	if message.Content == nil && len(message.ToolCalls) == 0 {
		return nil
	}
	return message
}
