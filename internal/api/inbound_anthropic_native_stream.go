package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Native Anthropic streams retain signed thinking blocks, block ordering, and
// provider-specific SSE fields while still flowing through dispatcher fallback.
func (h *Handler) handleNativeAnthropicStream(c *gin.Context, stream <-chan *model.StreamEvent, first *model.StreamEvent, modelName, sessionID string, start time.Time) {
	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondAnthropicError(c, http.StatusInternalServerError, "Streaming unsupported", "api_error")
		return
	}
	inputTokens, outputTokens := 0, 0
	cachedReadTokens, cacheCreationTokens := 0, 0
	// A client can disappear before the closing message_delta that carries
	// output_tokens, and some providers close the stream without one. Accumulating
	// the emitted text lets the audit row bill what was generated instead of
	// recording zero output for a stream the provider charged for.
	accumulatedOutputChars := 0
	usageSource := "upstream"
	var ttftDuration time.Duration
	var firstContentSeen bool
	chatID, channel, statusCode := "", first.Channel, http.StatusOK
	var streamErr error
	defer func() {
		// Anthropic input_tokens excludes prompt-cache reads/writes; the audit
		// record must report the full prompt and surface cache hits separately.
		totalPrompt := inputTokens + cachedReadTokens + cacheCreationTokens
		if outputTokens == 0 && accumulatedOutputChars > 0 {
			outputTokens = estimatedStreamCompletionTokens(accumulatedOutputChars)
			usageSource = usageSourceEstimated
		}
		RecordUsage(c, AuditRecordParams{
			SessionID:        sessionID,
			ChatID:           chatID,
			Channel:          channel,
			Model:            modelName,
			PromptTokens:     totalPrompt,
			CompletionTokens: outputTokens,
			CachedTokens:     cachedReadTokens,
			// Cache writes cost a premium over the plain input rate; passing them
			// separately stops them being billed as ordinary prompt tokens.
			CacheCreationTokens: cacheCreationTokens,
			Duration:            time.Since(start),
			TTFT:                ttftDuration,
			StatusCode:          statusCode,
			UsageSource:         usageSource,
			ErrorMessage:        sanitizeErrorMessage(streamErr),
			LiveTracked:         true, // stream deltas already fed the live token meter
		})
	}()
	write := func(event *model.StreamEvent) bool {
		if event == nil {
			return true
		}
		if event.Channel != "" {
			channel = event.Channel
		}
		if event.Err != nil {
			streamErr = event.Err
			statusCode = http.StatusBadGateway
			body, _ := json.Marshal(gin.H{"type": "error", "error": gin.H{"type": "stream_error", "message": sanitizeErrorMessage(event.Err)}})
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", body)
			flusher.Flush()
			return false
		}
		if len(event.NativeAnthropicEvent) == 0 {
			return true
		}
		var payload map[string]any
		if json.Unmarshal(event.NativeAnthropicEvent, &payload) == nil {
			if !firstContentSeen && (event.NativeAnthropicType == "content_block_delta" || event.NativeAnthropicType == "content_block_start") {
				ttftDuration = time.Since(start)
				telemetry.GlobalMetrics.RecordTTFT(ttftDuration)
				firstContentSeen = true
			}
			if event.NativeAnthropicType == "message_start" {
				if msg, ok := payload["message"].(map[string]any); ok {
					chatID, _ = msg["id"].(string)
					msg["model"] = modelName
					if usage, ok := msg["usage"].(map[string]any); ok {
						inputTokens = intFromJSONNumber(usage["input_tokens"])
						cachedReadTokens = intFromJSONNumber(usage["cache_read_input_tokens"])
						cacheCreationTokens = intFromJSONNumber(usage["cache_creation_input_tokens"])
					}
				}
			} else if event.NativeAnthropicType == "content_block_delta" {
				// 实时吞吐：从原生 delta 中取出文本增量计入内存 token 计数。
				if delta, ok := payload["delta"].(map[string]any); ok {
					if text, ok := delta["text"].(string); ok && text != "" {
						telemetry.GlobalLiveMetrics.AddChars(len(text))
						accumulatedOutputChars += len(text)
					}
				}
			} else if event.NativeAnthropicType == "message_delta" {
				if usage, ok := payload["usage"].(map[string]any); ok {
					outputTokens = intFromJSONNumber(usage["output_tokens"])
					if value, exists := usage["input_tokens"]; exists {
						inputTokens = intFromJSONNumber(value)
					}
					if value, exists := usage["cache_read_input_tokens"]; exists {
						cachedReadTokens = intFromJSONNumber(value)
					}
					if value, exists := usage["cache_creation_input_tokens"]; exists {
						cacheCreationTokens = intFromJSONNumber(value)
					}
				}
			}
			event.NativeAnthropicEvent, _ = json.Marshal(payload)
		}
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.NativeAnthropicType, event.NativeAnthropicEvent)
		flusher.Flush()
		return true
	}
	if !write(first) {
		return
	}
	clientGone := false
	drainEnabled := upstreamDrainingEnabled()
	doneCh := c.Request.Context().Done()
	for {
		select {
		case <-doneCh:
			// 499 matches the other stream handlers: the *client* went away, the
			// upstream did not time out.
			statusCode = 499
			if !drainEnabled {
				return
			}
			// Keep reading so the provider's message_delta usage still arrives.
			clientGone = true
			doneCh = nil
			markAbortedDrain(0)
		case event, open := <-stream:
			if !open {
				if clientGone {
					markAbortedDrain(outputTokens)
				}
				return
			}
			// write() also accumulates usage; emitting to a dead client is harmless,
			// so keep calling it but ignore write failures while draining.
			if ok := write(event); !ok && !clientGone {
				return
			}
		}
	}
}

func intFromJSONNumber(value any) int {
	if number, ok := value.(float64); ok {
		return int(number)
	}
	return 0
}
