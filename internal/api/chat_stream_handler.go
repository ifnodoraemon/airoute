package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// handleChatStream orchestrates SSE response streaming, token consumption metrics, and audit logging.
func (h *Handler) handleChatStream(c *gin.Context, reqCtx context.Context, req *model.ChatCompletionRequest, sessionID string, start time.Time) {
	// Track the in-flight stream for the dashboard's live telemetry gauges; the
	// defer keeps the gauge balanced on every exit path (error, disconnect).
	telemetry.GlobalLiveMetrics.BeginStream()
	defer telemetry.GlobalLiveMetrics.EndStream()

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

	// Prompt fallback for a stream that never reports usage. The canonical
	// estimator covers message text, multimodal parts and the tools block, so a
	// tool-heavy stream is billed on the same basis as the same unary request.
	// The old per-message character count dropped tools entirely.
	canonicalPromptEstimate := estimateCanonicalPromptTokens(req)

	var recordOnce sync.Once
	var streamError error
	usageSource := "upstream"
	var finalCost, finalSaved float64
	var finalCached int
	recordStreamEnd := func() {
		dur := time.Since(start)
		statusCode := http.StatusOK
		// The customer leaving is the truthful status regardless of how many
		// tokens the provider produced (a drained abort has a full usage report).
		if ctxErr := c.Request.Context().Err(); ctxErr != nil {
			statusCode = statusClientClosed
			if totalPromptTokens == 0 && canonicalPromptEstimate > 0 {
				// Nothing was reported: bill the estimated prompt the provider
				// consumed instead of recording a free request.
				totalPromptTokens = streamFallbackPromptTokens(req)
				usageSource = usageSourceEstimated
			}
		} else if streamError != nil {
			statusCode = http.StatusBadGateway
		} else {
			// Fallback token estimation if upstream provider did not report usage on successful completion
			if totalPromptTokens == 0 && canonicalPromptEstimate > 0 {
				totalPromptTokens = streamFallbackPromptTokens(req)
				usageSource = "estimated"
			} else if totalPromptTokens > 0 && canonicalPromptEstimate > 0 {
				observePromptEstimate(req.Model, canonicalPromptEstimate, totalPromptTokens)
			}
			if totalCompTokens == 0 && accumulatedCompChars > 0 {
				totalCompTokens = estimatedStreamCompletionTokens(accumulatedCompChars)
				usageSource = "estimated"
			}
		}

		finalCost, finalSaved, finalCached = recordUsage(c, AuditRecordParams{
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
			UsageSource:      usageSource,
			LiveTracked:      true, // stream deltas already fed the live token meter
			ErrorMessage: func() string {
				if streamError != nil {
					return streamError.Error()
				}
				return ""
			}(),
		})
	}
	defer recordOnce.Do(recordStreamEnd)

	w := c.Writer
	// Draining: when the customer disconnects we keep consuming the provider's
	// stream so its final usage report still arrives and the charge matches the
	// provider invoice exactly, instead of being estimated.
	clientGone := false
	drainEnabled := upstreamDrainingEnabled()
	doneCh := c.Request.Context().Done()
	for {
		select {
		case <-doneCh:
			if !drainEnabled {
				return
			}
			clientGone = true
			doneCh = nil // the channel is closed; stop selecting on it
			telemetry.ScopedLogger("billing").Info("downstream disconnected; draining the provider stream for exact usage",
				"trace_id", middleware.GetTraceID(c), "model", req.Model)
			markAbortedDrain(0)
		case event, open := <-streamChan:
			if !open {
				recordOnce.Do(recordStreamEnd)
				if !clientGone {
					writeStreamUsageTrailer(w, flusher, finalCost, finalSaved, finalCached)
					fmt.Fprintf(w, "data: [DONE]\n\n")
					flusher.Flush()
				} else {
					markAbortedDrain(totalCompTokens)
				}
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
				if clientGone {
					// The disconnect is not a provider failure for billing purposes:
					// keep the usage already collected and stop writing.
					telemetry.GlobalMetrics.AbortedDrainedCompletionTokens.Add(uint64(max(0, totalCompTokens)))
					recordOnce.Do(recordStreamEnd)
					return
				}
				streamError = event.Err
				telemetry.ScopedLogger("router").Error("stream error received mid-flight", "error", event.Err.Error())
				errJSON, _ := json.Marshal(gin.H{
					"error": gin.H{
						"message": sanitizeErrorMessage(event.Err),
						"type":    "stream_error",
					},
				})
				fmt.Fprintf(w, "data: %s\n\n", errJSON)
				recordOnce.Do(recordStreamEnd)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if event.IsDone {
				if clientGone {
					// Count what the provider generated (and billed) after the
					// customer left; this is the extra spend exact billing buys.
					telemetry.GlobalMetrics.AbortedDrainedCompletionTokens.Add(uint64(max(0, totalCompTokens)))
					recordOnce.Do(recordStreamEnd)
					return
				}
				recordOnce.Do(recordStreamEnd)
				writeStreamUsageTrailer(w, flusher, finalCost, finalSaved, finalCached)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
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
					if delta.Content != "" || delta.Role != "" || delta.ReasoningContent != "" {
						ttftDuration = time.Since(start)
						telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
						firstTokenRecorded = true
					}
				}
				if len(event.Chunk.Choices) > 0 {
					delta := event.Chunk.Choices[0].Delta
					deltaChars := 0
					if delta.Content != "" {
						deltaChars += len(delta.Content)
					}
					if delta.ReasoningContent != "" {
						deltaChars += len(delta.ReasoningContent)
					}
					if deltaChars > 0 {
						accumulatedCompChars += deltaChars
						// 实时吞吐：delta 到达即计入内存 token 计数，无需等待 usage 帧。
						telemetry.GlobalLiveMetrics.AddChars(deltaChars)
					}
				}

				if event.Chunk.Usage != nil {
					// Usage counters are cumulative; output-only trailers must not
					// erase input/cache counts reported by an earlier frame.
					totalPromptTokens = max(totalPromptTokens, event.Chunk.Usage.PromptTokens)
					totalCompTokens = max(totalCompTokens, event.Chunk.Usage.CompletionTokens)
					totalCachedTokens = max(totalCachedTokens, event.Chunk.Usage.GetCachedTokens())
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
