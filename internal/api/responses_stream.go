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

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
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

	for event := range streamChan {
		if respUpstreamChannel == "" && event.Channel != "" {
			respUpstreamChannel = event.Channel
		}

		if event.Err != nil {
			writeSSEJSON(w, flusher, "error", gin.H{
				"type": "error",
				"error": gin.H{
					"message": event.Err.Error(),
					"type":    "stream_error",
				},
			})
			break
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
	dur := time.Since(start)
	if totalPromptTokens == 0 && totalCompTokens == 0 {
		totalCompTokens = len(fullText) / 4
		if totalCompTokens == 0 && len(fullText) > 0 {
			totalCompTokens = 1
		}
		totalPromptTokens = 10
	}

	RecordUsage(c, AuditRecordParams{
		ChatID:           respUpstreamChatID,
		Channel:          respUpstreamChannel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     totalPromptTokens,
		CompletionTokens: totalCompTokens,
		Duration:         dur,
		TTFT:             ttftDuration,
		StatusCode:       http.StatusOK,
	})

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
