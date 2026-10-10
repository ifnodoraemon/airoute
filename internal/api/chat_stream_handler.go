package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// handleChatStream orchestrates SSE response streaming, token consumption metrics, and audit logging.
func (h *Handler) handleChatStream(c *gin.Context, reqCtx context.Context, req *model.ChatCompletionRequest, sessionID string, start time.Time) {
	if req.StreamOptions == nil {
		req.StreamOptions = &model.StreamOptions{IncludeUsage: true}
	} else {
		req.StreamOptions.IncludeUsage = true
	}

	streamChan, err := h.dispatcher.DispatchStream(reqCtx, req)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}

	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondOpenAIError(c, http.StatusInternalServerError, "Streaming unsupported by response writer", "server_error", "streaming_unsupported")
		return
	}

	firstTokenRecorded := false
	var ttftDuration time.Duration
	totalPromptTokens := 0
	totalCompTokens := 0
	totalCachedTokens := 0
	accumulatedCompChars := 0
	upstreamChatID := ""  // real upstream response ID, captured from the first chunk (may stay empty)
	upstreamChannel := "" // populated from the first stream event

	approxPromptChars := 0
	for _, m := range req.Messages {
		approxPromptChars += len(m.GetContentString())
	}

	var recordOnce sync.Once
	var streamError error
	recordStreamEnd := func() {
		dur := time.Since(start)
		statusCode := http.StatusOK
		if streamError != nil || (c.Request.Context().Err() != nil && totalCompTokens == 0 && accumulatedCompChars == 0) {
			statusCode = http.StatusBadGateway
			if c.Request.Context().Err() != nil {
				statusCode = 499
			}
			if totalCompTokens == 0 && accumulatedCompChars == 0 {
				totalPromptTokens = 0
			}
		} else {
			// Fallback token estimation if upstream provider did not report usage on successful completion
			if totalPromptTokens == 0 && approxPromptChars > 0 {
				totalPromptTokens = approxPromptChars / 3
				if totalPromptTokens < 1 {
					totalPromptTokens = 1
				}
			}
			if totalCompTokens == 0 && accumulatedCompChars > 0 {
				totalCompTokens = accumulatedCompChars / 3
				if totalCompTokens < 1 {
					totalCompTokens = 1
				}
			}
		}

		RecordUsage(c, AuditRecordParams{
			ChatID:           upstreamChatID,
			Channel:          upstreamChannel,
			SessionID:        sessionID,
			Model:            req.Model,
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompTokens,
			CachedTokens:     totalCachedTokens,
			Duration:         dur,
			TTFT:             ttftDuration,
			StatusCode:       statusCode,
		})
	}
	defer recordOnce.Do(recordStreamEnd)

	w := c.Writer
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-streamChan:
			if !open {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				recordOnce.Do(recordStreamEnd)
				return
			}

			// Capture the channel that actually serves this stream BEFORE
			// any short-circuit: terminal events (IsDone/Err) return before
			// the rest of the loop body, and a stream that ends on its very
			// first event would lose the stamp. Dispatcher stamps every event.
			if upstreamChannel == "" && event.Channel != "" {
				upstreamChannel = event.Channel
			}

			if event.Err != nil {
				streamError = event.Err
				telemetry.Logger.Error("stream error received mid-flight", "error", event.Err.Error())
				errJSON, _ := json.Marshal(gin.H{
					"error": gin.H{
						"message": event.Err.Error(),
						"type":    "stream_error",
					},
				})
				fmt.Fprintf(w, "data: %s\n\n", errJSON)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if event.IsDone {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				recordOnce.Do(recordStreamEnd)
				return
			}

			if event.Chunk != nil {
				if req.Model != "" {
					event.Chunk.Model = req.Model
				}
				// Capture the upstream provider's response ID from the first chunk
				if event.Chunk.ID != "" && upstreamChatID == "" {
					upstreamChatID = event.Chunk.ID
				}
				if !firstTokenRecorded && len(event.Chunk.Choices) > 0 {
					delta := event.Chunk.Choices[0].Delta
					if delta.Content != "" || delta.Role != "" {
						ttftDuration = time.Since(start)
						telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
						firstTokenRecorded = true
					}
				}
				if len(event.Chunk.Choices) > 0 && event.Chunk.Choices[0].Delta.Content != "" {
					accumulatedCompChars += len(event.Chunk.Choices[0].Delta.Content)
				}

				if event.Chunk.Usage != nil {
					totalPromptTokens = event.Chunk.Usage.PromptTokens
					totalCompTokens = event.Chunk.Usage.CompletionTokens
					totalCachedTokens = event.Chunk.Usage.GetCachedTokens()
				}

				chunkBytes, err := json.Marshal(event.Chunk)
				if err == nil {
					fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
					flusher.Flush()
				}
			} else if len(event.Raw) > 0 {
				w.Write(event.Raw)
				flusher.Flush()
			}
		}
	}
}
