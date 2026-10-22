package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func (h *Handler) handleCompletionsStream(c *gin.Context, reqCtx context.Context, req LegacyCompletionRequest, chatReq model.ChatCompletionRequest, sessionID string, start time.Time) {
	// Track the in-flight stream for the dashboard's live telemetry gauges.
	telemetry.GlobalLiveMetrics.BeginStream()
	defer telemetry.GlobalLiveMetrics.EndStream()

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
	firstTokenRecorded := false
	var ttftDuration time.Duration
	respID := fmt.Sprintf("cmpl_%d", time.Now().UnixMicro())
	nowCreated := time.Now().Unix()

	totalPromptTokens := 0
	totalCompTokens := 0
	upstreamChannel := ""
	accumulatedChars := 0
	var streamError error
	sawDone := false
	usageSource := "upstream"

	for event := range streamChan {
		if upstreamChannel == "" && event.Channel != "" {
			upstreamChannel = event.Channel
		}
		if event.Err != nil {
			streamError = event.Err
			errBytes, _ := json.Marshal(gin.H{
				"error": gin.H{
					"message": sanitizeErrorMessage(event.Err),
					"type":    "stream_error",
				},
			})
			fmt.Fprintf(w, "data: %s\n\n", errBytes)
			flusher.Flush()
			break
		}

		if event.IsDone {
			sawDone = true
			break
		}

		if event.Chunk != nil {
			// Preserve every choice with its real index; `n > 1` used to collapse
			// to choices[0] renumbered as 0.
			choices := make([]gin.H, 0, len(event.Chunk.Choices))
			for _, ch := range event.Chunk.Choices {
				if ch.Delta.Content != "" {
					if !firstTokenRecorded {
						ttftDuration = time.Since(start)
						telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
						firstTokenRecorded = true
					}
					accumulatedChars += len(ch.Delta.Content)
					// 实时吞吐：流式 delta 到达即计入内存 token 计数。
					telemetry.GlobalLiveMetrics.AddChars(len(ch.Delta.Content))
				}
				choices = append(choices, gin.H{
					"text":          ch.Delta.Content,
					"index":         ch.Index,
					"logprobs":      nil,
					"finish_reason": ch.FinishReason,
				})
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
				"choices": choices,
			}
			chunkBytes, err := json.Marshal(chunkPayload)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
				flusher.Flush()
			}
		}
	}

	statusCode := http.StatusOK
	if streamError != nil {
		statusCode = http.StatusBadGateway
		if c.Request.Context().Err() != nil {
			statusCode = 499
		}
	} else {
		if totalPromptTokens == 0 {
			totalPromptTokens = estimateCanonicalPromptTokens(&chatReq)
			usageSource = "estimated"
		}
		if totalCompTokens == 0 && accumulatedChars > 0 {
			totalCompTokens = estimateTokens(accumulatedChars)
			usageSource = "estimated"
		}
	}

	cost, savedCost, cachedTokens := recordUsage(c, AuditRecordParams{
		ChatID:           respID,
		Channel:          upstreamChannel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     totalPromptTokens,
		CompletionTokens: totalCompTokens,
		Duration:         time.Since(start),
		TTFT:             ttftDuration,
		StatusCode:       statusCode,
		UsageSource:      usageSource,
		LiveTracked:      true, // stream deltas already fed the live token meter
		ErrorMessage:     sanitizeErrorMessage(streamError),
	})

	if statusCode == http.StatusOK {
		writeStreamUsageTrailer(w, flusher, cost, savedCost, cachedTokens)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	} else if sawDone {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}
