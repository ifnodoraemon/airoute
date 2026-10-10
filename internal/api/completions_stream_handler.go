package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

func (h *Handler) handleCompletionsStream(c *gin.Context, reqCtx context.Context, req LegacyCompletionRequest, chatReq model.ChatCompletionRequest, sessionID string, start time.Time) {
	streamChan, err := h.dispatcher.DispatchStream(reqCtx, &chatReq)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}

	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondOpenAIError(c, http.StatusInternalServerError, "Streaming unsupported", "server_error", "streaming_unsupported")
		return
	}

	w := c.Writer
	respID := fmt.Sprintf("cmpl_%d", time.Now().UnixNano())
	nowCreated := time.Now().Unix()

	totalPromptTokens := 0
	totalCompTokens := 0
	upstreamChannel := ""
	accumulatedChars := 0

	for event := range streamChan {
		if upstreamChannel == "" && event.Channel != "" {
			upstreamChannel = event.Channel
		}
		if event.Err != nil {
			errBytes, _ := json.Marshal(gin.H{
				"error": gin.H{
					"message": event.Err.Error(),
					"type":    "stream_error",
				},
			})
			fmt.Fprintf(w, "data: %s\n\n", errBytes)
			flusher.Flush()
			break
		}

		if event.IsDone {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}

		if event.Chunk != nil {
			deltaText := ""
			var finishReason *string
			if len(event.Chunk.Choices) > 0 {
				deltaText = event.Chunk.Choices[0].Delta.Content
				finishReason = event.Chunk.Choices[0].FinishReason
			}
			if deltaText != "" {
				accumulatedChars += len(deltaText)
			}
			if event.Chunk.Usage != nil {
				totalPromptTokens = event.Chunk.Usage.PromptTokens
				totalCompTokens = event.Chunk.Usage.CompletionTokens
			}

			chunkPayload := gin.H{
				"id":      respID,
				"object":  "text_completion",
				"created": nowCreated,
				"model":   req.Model,
				"choices": []gin.H{
					{
						"text":          deltaText,
						"index":         0,
						"logprobs":      nil,
						"finish_reason": finishReason,
					},
				},
			}
			chunkBytes, err := json.Marshal(chunkPayload)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
				flusher.Flush()
			}
		}
	}

	dur := time.Since(start)
	if totalPromptTokens == 0 && totalCompTokens == 0 {
		totalCompTokens = accumulatedChars / 4
		if totalCompTokens == 0 && accumulatedChars > 0 {
			totalCompTokens = 1
		}
		totalPromptTokens = 10
	}
	RecordUsage(c, AuditRecordParams{
		ChatID:           respID,
		Channel:          upstreamChannel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     totalPromptTokens,
		CompletionTokens: totalCompTokens,
		Duration:         dur,
		StatusCode:       http.StatusOK,
	})
}
