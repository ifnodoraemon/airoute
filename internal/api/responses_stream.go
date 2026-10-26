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

	// 1. response.created
	writeSSEJSON(w, flusher, "response.created", gin.H{
		"type": "response.created",
		"response": gin.H{
			"id":         respID,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"status":     "in_progress",
			"model":      req.Model,
		},
	})

	// 2. response.output_item.added
	writeSSEJSON(w, flusher, "response.output_item.added", gin.H{
		"type": "response.output_item.added",
		"output_item": gin.H{
			"id":      msgID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []any{},
		},
	})

	// 3. response.content_part.added
	writeSSEJSON(w, flusher, "response.content_part.added", gin.H{
		"type": "response.content_part.added",
		"part": gin.H{
			"type": "output_text",
			"text": "",
		},
	})

	var fullContent strings.Builder
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
			if totalPromptTokens == 0 {
				totalPromptTokens = calibratedPromptEstimate(chatReq.Model, estimateCanonicalPromptTokens(chatReq))
			} else {
				observePromptEstimate(chatReq.Model, estimateCanonicalPromptTokens(chatReq), totalPromptTokens)
			}
			if totalCompTokens == 0 && fullContent.Len() > 0 {
				totalCompTokens = estimateTokens(fullContent.Len())
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
			writeSSEJSON(w, flusher, "error", gin.H{
				"type": "error",
				"error": gin.H{
					"message": sanitizeErrorMessage(event.Err),
					"type":    "stream_error",
				},
			})
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
					fullContent.WriteString(deltaText)
					// 实时吞吐：流式 delta 到达即计入内存 token 计数。
					telemetry.GlobalLiveMetrics.AddChars(len(deltaText))
					writeSSEJSON(w, flusher, "response.output_text.delta", gin.H{
						"type":  "response.output_text.delta",
						"delta": deltaText,
					})
				}
			}
		}
	}

	fullText := fullContent.String()

	// response.output_text.done
	writeSSEJSON(w, flusher, "response.output_text.done", gin.H{
		"type": "response.output_text.done",
		"text": fullText,
	})

	// response.completed
	cost, savedCost, cachedTokens := recordStreamEnd(http.StatusOK)
	writeStreamUsageTrailer(w, flusher, cost, savedCost, cachedTokens)

	writeSSEJSON(w, flusher, "response.completed", gin.H{
		"type": "response.completed",
		"response": gin.H{
			"id":         respID,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"status":     "completed",
			"model":      req.Model,
			"output": []gin.H{
				{
					"id":     msgID,
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []gin.H{
						{
							"type": "output_text",
							"text": fullText,
						},
					},
				},
			},
			"usage": gin.H{
				"input_tokens":  totalPromptTokens,
				"output_tokens": totalCompTokens,
				"total_tokens":  totalPromptTokens + totalCompTokens,
			},
		},
	})

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
