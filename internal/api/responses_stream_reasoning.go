package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// reasoningDelta streams one upstream reasoning increment as the documented
// Responses reasoning-summary events. The adapted path used to count reasoning
// bytes for billing only and drop the text, so a thinking model served by a chat
// upstream produced no reasoning output at all. The reasoning output item is
// allocated the next output_index on first sight (before the message item, which
// is announced lazily), matching the native Responses ordering.
func (e *responsesStreamEmitter) reasoningDelta(delta string) {
	if e.reasoningOutputIndex < 0 {
		e.reasoningOutputIndex = e.nextOutputIndex
		e.nextOutputIndex++
		e.reasoningItemID = fmt.Sprintf("rs_%s", e.respID)
		e.emit("response.output_item.added", gin.H{
			"output_index": e.reasoningOutputIndex,
			"item":         e.reasoningItem("in_progress", ""),
		})
	}
	if !e.reasoningPartAdded {
		e.reasoningPartAdded = true
		e.emit("response.reasoning_summary_part.added", gin.H{
			"item_id":       e.reasoningItemID,
			"output_index":  e.reasoningOutputIndex,
			"summary_index": 0,
			"part":          gin.H{"type": "summary_text", "text": ""},
		})
	}
	e.reasoningText.WriteString(delta)
	e.reasoningChars += len(delta)
	e.emit("response.reasoning_summary_text.delta", gin.H{
		"item_id":       e.reasoningItemID,
		"output_index":  e.reasoningOutputIndex,
		"summary_index": 0,
		"delta":         delta,
	})
}

// reasoningItem renders the reasoning output item. The summary array is empty
// while in progress and carries the assembled summary text once completed.
func (e *responsesStreamEmitter) reasoningItem(status, text string) gin.H {
	summary := []gin.H{}
	if status == "completed" {
		summary = append(summary, gin.H{"type": "summary_text", "text": text})
	}
	return gin.H{
		"id":      e.reasoningItemID,
		"type":    "reasoning",
		"status":  status,
		"summary": summary,
	}
}

// closeReasoningItem emits the terminal reasoning events and returns the
// completed item for the response.completed output array.
func (e *responsesStreamEmitter) closeReasoningItem() gin.H {
	text := e.reasoningText.String()
	e.emit("response.reasoning_summary_text.done", gin.H{
		"item_id":       e.reasoningItemID,
		"output_index":  e.reasoningOutputIndex,
		"summary_index": 0,
		"text":          text,
	})
	e.emit("response.reasoning_summary_part.done", gin.H{
		"item_id":       e.reasoningItemID,
		"output_index":  e.reasoningOutputIndex,
		"summary_index": 0,
		"part":          gin.H{"type": "summary_text", "text": text},
	})
	item := e.reasoningItem("completed", text)
	e.emit("response.output_item.done", gin.H{
		"output_index": e.reasoningOutputIndex,
		"item":         item,
	})
	return item
}