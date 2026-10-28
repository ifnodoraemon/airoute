package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
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

// responsesStreamEmitter emits the documented OpenAI Responses API streaming
// events.
//
// A07: the previous implementation sent only {"type": ...} plus raw text, with no
// item_id/output_index/content_index/sequence_number and with the output item
// under "output_item" instead of "item". Official SDKs build the final response
// by correlating those fields, so a stream without them cannot be assembled and
// the client reports a malformed event (or silently produces no output).
type responsesStreamEmitter struct {
	w         gin.ResponseWriter
	flusher   http.Flusher
	sequence  int
	respID    string
	itemID    string
	model     string
	createdAt int64
	fullText  strings.Builder

	// Tool calls are additional output items after the message item (index 0).
	// The upstream chat delta identifies them by its own index; the Responses
	// output_index is allocated in first-seen order.
	toolStates      map[int]*responsesToolCallState
	toolOrder       []int
	nextOutputIndex int
	toolChars       int
	// finishReason is the upstream chat finish_reason, used to report a truncated
	// or filtered answer as an "incomplete" response instead of "completed".
	finishReason string
}

type responsesToolCallState struct {
	outputIndex int
	itemID      string
	callID      string
	name        string
	arguments   strings.Builder
}

func newResponsesStreamEmitter(w gin.ResponseWriter, flusher http.Flusher, respID, itemID, modelStr string) *responsesStreamEmitter {
	return &responsesStreamEmitter{
		w:               w,
		flusher:         flusher,
		respID:          respID,
		itemID:          itemID,
		model:           modelStr,
		createdAt:       time.Now().Unix(),
		toolStates:      make(map[int]*responsesToolCallState),
		nextOutputIndex: 1, // 0 is reserved for the message item
	}
}

// nextSequence returns the next event sequence number. Every documented event
// carries one and clients rely on it being monotonically increasing.
func (e *responsesStreamEmitter) nextSequence() int {
	seq := e.sequence
	e.sequence++
	return seq
}

// emit writes one named event with the sequence number attached.
func (e *responsesStreamEmitter) emit(event string, payload gin.H) {
	payload["type"] = event
	payload["sequence_number"] = e.nextSequence()
	writeSSEJSON(e.w, e.flusher, event, payload)
}

// responseObject renders the response envelope shared by created/in_progress/
// completed.
func (e *responsesStreamEmitter) responseObject(status string, output []gin.H, usage gin.H) gin.H {
	obj := gin.H{
		"id":         e.respID,
		"object":     "response",
		"created_at": e.createdAt,
		"status":     status,
		"model":      e.model,
		"output":     output,
	}
	if usage != nil {
		obj["usage"] = usage
	}
	if status != model.ResponseStatusCompleted {
		if _, details := model.IncompleteResponseStatus(e.finishReason); details != nil {
			obj["incomplete_details"] = gin.H{"reason": details.Reason}
		}
	}
	return obj
}

// messageItem renders the assistant message output item.
func (e *responsesStreamEmitter) messageItem(status string, content []gin.H) gin.H {
	return gin.H{
		"id":      e.itemID,
		"type":    "message",
		"status":  status,
		"role":    "assistant",
		"content": content,
	}
}

// contentPart renders an output_text part.
func (e *responsesStreamEmitter) contentPart(text string) gin.H {
	return gin.H{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
	}
}

// functionCallItem renders a function_call output item.
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

// created emits response.created and response.in_progress.
func (e *responsesStreamEmitter) created() {
	e.emit("response.created", gin.H{
		"response": e.responseObject("in_progress", []gin.H{}, nil),
	})
	e.emit("response.in_progress", gin.H{
		"response": e.responseObject("in_progress", []gin.H{}, nil),
	})
}

// itemAdded emits response.output_item.added and response.content_part.added for
// the single text output item this gateway streams.
func (e *responsesStreamEmitter) itemAdded() {
	e.emit("response.output_item.added", gin.H{
		"output_index": 0,
		"item":         e.messageItem("in_progress", []gin.H{}),
	})
	e.emit("response.content_part.added", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"part":          e.contentPart(""),
	})
}

// textDelta emits one response.output_text.delta event.
func (e *responsesStreamEmitter) textDelta(delta string) {
	e.emit("response.output_text.delta", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"delta":         delta,
		"logprobs":      []any{},
	})
}

// toolCallDelta appends one upstream tool-call delta and emits the Responses
// events that describe it: a function_call output item on first sight, then one
// arguments delta per increment.
func (e *responsesStreamEmitter) toolCallDelta(toolIndex int, callID, name, arguments string) {
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

// noteFinishReason records the upstream chat finish_reason so the terminal
// response.completed event can report status "incomplete" (with
// incomplete_details) instead of claiming a truncated answer completed normally.
func (e *responsesStreamEmitter) noteFinishReason(finishReason string) {
	if finishReason != "" {
		e.finishReason = finishReason
	}
}

// terminalStatus maps the recorded finish_reason onto the Responses API status.
func (e *responsesStreamEmitter) terminalStatus() string {
	status, _ := model.IncompleteResponseStatus(e.finishReason)
	return status
}

// completed emits the terminal events (message done, every function_call done,
// response.completed) that close the stream. Output items are ordered by their
// output_index: the message item first, then function calls in first-seen order.
func (e *responsesStreamEmitter) completed(usage gin.H) {
	text := e.fullText.String()
	e.emit("response.output_text.done", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"text":          text,
		"logprobs":      []any{},
	})
	e.emit("response.content_part.done", gin.H{
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"part":          e.contentPart(text),
	})
	output := make([]gin.H, 0, 1+len(e.toolOrder))
	message := e.messageItem("completed", []gin.H{e.contentPart(text)})
	e.emit("response.output_item.done", gin.H{
		"output_index": 0,
		"item":         message,
	})
	output = append(output, message)

	for _, toolIndex := range e.toolOrder {
		state := e.toolStates[toolIndex]
		if state == nil {
			continue
		}
		arguments := state.arguments.String()
		e.emit("response.function_call_arguments.done", gin.H{
			"item_id":      state.itemID,
			"output_index": state.outputIndex,
			"arguments":    arguments,
		})
		item := e.functionCallItem("completed", state.itemID, state.callID, state.name, arguments)
		e.emit("response.output_item.done", gin.H{
			"output_index": state.outputIndex,
			"item":         item,
		})
		output = append(output, item)
	}

	e.emit("response.completed", gin.H{
		"response": e.responseObject(e.terminalStatus(), output, usage),
	})
}

// failed emits the documented error event. The nested "error" object is kept for
// clients that predate the documented shape.
func (e *responsesStreamEmitter) failed(err error) {
	message := sanitizeErrorMessage(err)
	e.emit("error", gin.H{
		"code":    "upstream_error",
		"message": message,
		"param":   nil,
		"error": gin.H{
			"message": message,
			"type":    "stream_error",
		},
	})
}
