package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// emitAnthropicToolBlocks renders assembled tool calls as sequential Anthropic
// tool_use content blocks (start + input_json_delta + stop) and returns the last
// block index used. OpenAI interleaves parallel tool_call fragments by index
// while Anthropic content blocks are strictly sequential, so the calls are
// buffered by the renderer and emitted here in first-seen order with their
// complete arguments instead of streaming each fragment onto whichever block
// happens to be open.
func emitAnthropicToolBlocks(w http.ResponseWriter, flusher http.Flusher, tools *geminiStreamToolCalls, lastIndex int) int {
	for _, idx := range tools.order {
		call := tools.calls[idx]
		if call == nil {
			continue
		}
		lastIndex++
		tbStart := gin.H{
			"type":  "content_block_start",
			"index": lastIndex,
			"content_block": gin.H{
				"type":  "tool_use",
				"id":    call.id,
				"name":  call.name,
				"input": gin.H{},
			},
		}
		tbBytes, _ := json.Marshal(tbStart)
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", tbBytes)
		args := call.args.String()
		if args == "" {
			args = "{}"
		}
		argDelta := gin.H{
			"type":  "content_block_delta",
			"index": lastIndex,
			"delta": gin.H{"type": "input_json_delta", "partial_json": args},
		}
		argBytes, _ := json.Marshal(argDelta)
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", argBytes)
		fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", lastIndex)
		flusher.Flush()
	}
	return lastIndex
}

// emitAnthropicStreamStart writes the message_start and initial text
// content_block_start frames that open an adapted Anthropic stream. Extracted
// from handleAnthropicStream to keep it under the size red line.
func emitAnthropicStreamStart(w http.ResponseWriter, flusher http.Flusher, msgID, modelName string, inputTokens int) {
	startEvent := gin.H{
		"type": "message_start",
		"message": gin.H{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         modelName,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": gin.H{
				"input_tokens":  inputTokens,
				"output_tokens": 1,
			},
		},
	}
	startBytes, _ := json.Marshal(startEvent)
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", startBytes)

	blockStart := gin.H{
		"type":  "content_block_start",
		"index": 0,
		"content_block": gin.H{
			"type": "text",
			"text": "",
		},
	}
	blockStartBytes, _ := json.Marshal(blockStart)
	fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", blockStartBytes)
	flusher.Flush()
}

// recordAnthropicStreamEnd estimates missing usage (mutating the caller's counters
// so a later message_delta reflects the estimate) and enqueues the audit record.
// Extracted from handleAnthropicStream to keep it under the size red line.
func recordAnthropicStreamEnd(c *gin.Context, statusCode int, promptTokens, compTokens *int, usageSource *string, accumulatedChars int, canonicalReq *model.ChatCompletionRequest, params AuditRecordParams) (float64, float64, int, func()) {
	// A client abort still consumed the prompt upstream: the request had been
	// dispatched before the disconnect, so bill the estimated prompt instead of
	// recording zero.
	if statusCode == http.StatusOK || statusCode == statusClientClosed {
		// Anthropic wire format requires input_tokens on every message. When the
		// upstream stream omits usage, approximate the prompt from the request so
		// the audit log does not show a misleading zero.
		if *promptTokens == 0 {
			if estimated := streamFallbackPromptTokens(canonicalReq); estimated > 0 {
				*promptTokens = estimated
				*usageSource = "estimated"
			}
		} else {
			observePromptEstimate(canonicalReq.Model, estimateCanonicalPromptTokens(canonicalReq), *promptTokens)
		}
		if *compTokens == 0 && accumulatedChars > 0 {
			*compTokens = estimatedStreamCompletionTokens(accumulatedChars)
			*usageSource = "estimated"
		}
	}
	params.PromptTokens = *promptTokens
	params.CompletionTokens = *compTokens
	params.UsageSource = *usageSource
	return RecordUsageDeferred(c, params)
}

// emitAnthropicTextDelta opens a text content block if needed and emits a
// text_delta frame. Extracted from handleAnthropicStream to keep it under the
// size red line. blockIdx/blockOpened are advanced in place.
func emitAnthropicTextDelta(w http.ResponseWriter, flusher http.Flusher, text string, blockIdx *int, blockOpened *bool) {
	if !*blockOpened {
		*blockIdx++
		*blockOpened = true
		tbStart := gin.H{"type": "content_block_start", "index": *blockIdx, "content_block": gin.H{"type": "text", "text": ""}}
		tbBytes, _ := json.Marshal(tbStart)
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", tbBytes)
	}
	blockDelta := gin.H{
		"type":  "content_block_delta",
		"index": *blockIdx,
		"delta": gin.H{"type": "text_delta", "text": text},
	}
	deltaBytes, _ := json.Marshal(blockDelta)
	fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", deltaBytes)
	flusher.Flush()
}

// emitAnthropicMessageDelta emits the terminal message_delta frame carrying the
// stop reason and usage. Extracted from handleAnthropicStream.
func emitAnthropicMessageDelta(w http.ResponseWriter, flusher http.Flusher, stopReason string, promptTokens, cachedTokens, compTokens int) {
	deltaEvent := gin.H{
		"type": "message_delta",
		"delta": gin.H{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": gin.H{
			"input_tokens":            max(0, promptTokens-cachedTokens),
			"cache_read_input_tokens": cachedTokens,
			"output_tokens":           compTokens,
		},
	}
	deltaBytes, _ := json.Marshal(deltaEvent)
	fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", deltaBytes)
}
