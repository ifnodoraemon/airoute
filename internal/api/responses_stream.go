package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// writeSSEJSON sends a named SSE event with JSON-encoded data and flushes the writer.
func writeSSEJSON(w gin.ResponseWriter, flusher http.Flusher, event string, data any) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	if event != "" {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, bytes)
	} else {
		fmt.Fprintf(w, "data: %s\n\n", bytes)
	}
	flusher.Flush()
}

// responsesStreamEmitter emits the documented OpenAI Responses API streaming
// events.
//
// A07: the previous implementation sent only {"type": ...} plus raw text, with no
// item_id/output_index/content_index/sequence_number and with the output item
// under "output_item" instead of "item". Official SDKs build the final response
// by correlating those fields, so a stream without them cannot be assembled and
// the client reports a malformed event (or silently produces no output).
type responsesStreamEmitter struct {
	w         gin.ResponseWriter
	flusher   http.Flusher
	sequence  int
	respID    string
	itemID    string
	model     string
	createdAt int64
	fullText  strings.Builder
}

func newResponsesStreamEmitter(w gin.ResponseWriter, flusher http.Flusher, respID, itemID, modelStr string) *responsesStreamEmitter {
	return &responsesStreamEmitter{
		w:         w,
		flusher:   flusher,
		respID:    respID,
		itemID:    itemID,
		model:     modelStr,
		createdAt: time.Now().Unix(),
	}
}

// nextSequence returns the next event sequence number. Every documented event
// carries one and clients rely on it being monotonically increasing.
func (e *responsesStreamEmitter) nextSequence() int {
	seq := e.sequence
	e.sequence++
	return seq
}

// emit writes one named event with the sequence number attached.
func (e *responsesStreamEmitter) emit(event string, payload gin.H) {
	payload["type"] = event
	payload["sequence_number"] = e.nextSequence()
	writeSSEJSON(e.w, e.flusher, event, payload)
}

// responseObject renders the response envelope shared by created/in_progress/
// completed.
func (e *responsesStreamEmitter) responseObject(status string, output []gin.H, usage gin.H) gin.H {
	obj := gin.H{
		"id":         e.respID,
		"object":     "response",
		"created_at": e.createdAt,
		"status":     status,
		"model":      e.model,
		"output":     output,
	}
	if usage != nil {
		obj["usage"] = usage
	}
	return obj
}

// messageItem renders the assistant message output item.
func (e *responsesStreamEmitter) messageItem(status string, content []gin.H) gin.H {
	return gin.H{
		"id":      e.itemID,
		"type":    "message",
		"status":  status,
		"role":    "assistant",
		"content": content,
	}
}

// contentPart renders an output_text part.
func (e *responsesStreamEmitter) contentPart(text string) gin.H {
	return gin.H{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
	}
}

// created emits response.created and response.in_progress.
func (e *responsesStreamEmitter) created() {
	e.emit("response.created", gin.H{
		"response": e.responseObject("in_progress", []gin.H{}, nil),
	})
	e.emit("response.in_progress", gin.H{
		"response": e.responseObject("in_progress", []gin.H{}, nil),
	})
}

// itemAdded emits response.output_item.added and response.content_part.added for
// the single text output item this gateway streams.
func (e *responsesStreamEmitter) itemAdded() {
	e.emit("response.output_item.added", gin.H{
		"output_index": 0,
		"item":         e.messageItem("in_progress", []gin.H{}),
	})
	e.emit("response.content_part.added", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"part":          e.contentPart(""),
	})
}

// textDelta emits one response.output_text.delta event.
func (e *responsesStreamEmitter) textDelta(delta string) {
	e.emit("response.output_text.delta", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"delta":         delta,
		"logprobs":      []any{},
	})
}

// completed emits the three terminal events (part done, item done, response
// completed) that close the stream.
func (e *responsesStreamEmitter) completed(usage gin.H) {
	text := e.fullText.String()
	e.emit("response.output_text.done", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"text":          text,
		"logprobs":      []any{},
	})
	e.emit("response.content_part.done", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"part":          e.contentPart(text),
	})
	item := e.messageItem("completed", []gin.H{e.contentPart(text)})
	e.emit("response.output_item.done", gin.H{
		"output_index": 0,
		"item":         item,
	})
	e.emit("response.completed", gin.H{
		"response": e.responseObject("completed", []gin.H{item}, usage),
	})
}

// failed emits the documented error event. The nested "error" object is kept for
// clients that predate the documented shape.
func (e *responsesStreamEmitter) failed(err error) {
	message := sanitizeErrorMessage(err)
	e.emit("error", gin.H{
		"code":    "upstream_error",
		"message": message,
		"param":   nil,
		"error": gin.H{
			"message": message,
			"type":    "stream_error",
		},
	})
}

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
			if totalCompTokens == 0 && emitter.fullText.Len() > 0 {
				totalCompTokens = estimatedStreamCompletionTokens(emitter.fullText.Len())
			}
		}
		return recordUsage(c, AuditRecordParams{
			ChatID:           respUpstreamChatID,
			Channel:          respUpstreamChannel,
			SessionID:        sessionID,
			Model:            req.Model,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			Duration:         time.Since(start),
			TTFT:             ttftDuration,
			StatusCode:       statusCode,
			ErrorMessage:     sanitizeErrorMessage(streamError),
			LiveTracked:      true, // stream deltas already fed the live token meter
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
			}

			for _, choice := range event.Chunk.Choices {
				deltaText := choice.Delta.Content
				if deltaText != "" {
					emitter.fullText.WriteString(deltaText)
					// 实时吞吐：流式 delta 到达即计入内存 token 计数。
					telemetry.GlobalLiveMetrics.AddChars(len(deltaText))
					emitter.textDelta(deltaText)
				}
			}
		}
	}

	cost, savedCost, cachedTokens := recordStreamEnd(http.StatusOK)

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
