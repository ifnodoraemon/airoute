package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// responsesToolCallState tracks one function_call output item being assembled
// across upstream chat deltas. Extracted from responses_stream_events.go to keep
// the emitter file under the size red line.
type responsesToolCallState struct {
	outputIndex int
	itemID      string
	callID      string
	name        string
	arguments   strings.Builder
}

func (e *responsesStreamEmitter) functionCallItem(status, itemID, callID, name, arguments string) gin.H {
	return gin.H{
		"id":        itemID,
		"type":      "function_call",
		"status":    status,
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
}

func (e *responsesStreamEmitter) toolCallDelta(toolIndex int, callID, name, arguments string) {
	e.ensureMessageItem()
	state := e.toolStates[toolIndex]
	if state == nil {
		state = &responsesToolCallState{
			outputIndex: e.nextOutputIndex,
			itemID:      fmt.Sprintf("fc_%s_%d", e.respID, toolIndex),
			callID:      callID,
			name:        name,
		}
		e.nextOutputIndex++
		e.toolStates[toolIndex] = state
		e.toolOrder = append(e.toolOrder, toolIndex)
		e.emit("response.output_item.added", gin.H{
			"output_index": state.outputIndex,
			"item":         e.functionCallItem("in_progress", state.itemID, state.callID, state.name, ""),
		})
	}
	// Some upstreams split the id/name across frames; keep the first non-empty
	// value (the item was already announced, but the completed item carries the
	// authoritative values).
	if state.callID == "" && callID != "" {
		state.callID = callID
	}
	if state.name == "" && name != "" {
		state.name = name
	}
	if arguments == "" {
		return
	}
	state.arguments.WriteString(arguments)
	e.toolChars += len(arguments)
	e.emit("response.function_call_arguments.delta", gin.H{
		"item_id":      state.itemID,
		"output_index": state.outputIndex,
		"delta":        arguments,
	})
}