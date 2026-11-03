package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// TestResponsesStreamEmitsFunctionCallEvents drives the Responses SSE emitter
// with an upstream chat stream that only produces a tool call, and verifies the
// official event contract: a function_call output item, incremental argument
// deltas, a done event, and a completed response that carries the item.
func TestResponsesStreamEmitsFunctionCallEvents(t *testing.T) {
	upstream := fixBatch2SSEServer(t,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"BJ\"}"}}]}}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":3}}`,
		`data: [DONE]`,
	)
	h := fixBatch2ChatHandler(t, upstream.URL)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"m","stream":true,"input":"hi","tools":[{"type":"function","name":"get_weather"}]}`))

	req := model.ResponseRequest{
		Model:  "m",
		Stream: true,
		Input:  "hi",
		Tools:  []any{map[string]any{"type": "function", "name": "get_weather"}},
	}
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	h.handleResponsesStreaming(c, c.Request.Context(), req, chatReq, "sess-tools", time.Now())

	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream is missing [DONE]:\n%s", body)
	}
	events := fixBatch2ParseSSE(body)

	var addedItems []map[string]any
	var argumentDeltas []string
	var argumentDone string
	var doneItem map[string]any
	var completedResponse map[string]any

	for _, ev := range events {
		switch ev.Event {
		case "response.output_item.added":
			item, _ := ev.Data["item"].(map[string]any)
			if item != nil && item["type"] == "function_call" {
				addedItems = append(addedItems, ev.Data)
			}
		case "response.function_call_arguments.delta":
			delta, _ := ev.Data["delta"].(string)
			argumentDeltas = append(argumentDeltas, delta)
		case "response.function_call_arguments.done":
			argumentDone, _ = ev.Data["arguments"].(string)
		case "response.output_item.done":
			item, _ := ev.Data["item"].(map[string]any)
			if item != nil && item["type"] == "function_call" {
				doneItem = item
			}
		case "response.completed":
			completedResponse, _ = ev.Data["response"].(map[string]any)
		}
	}

	if len(addedItems) != 1 {
		t.Fatalf("expected one function_call output_item.added, got %d:\n%s", len(addedItems), body)
	}
	added := addedItems[0]
	if idx, _ := added["output_index"].(float64); idx != 1 {
		t.Fatalf("function_call output_index = %v, want 1 (message item holds 0)", added["output_index"])
	}
	addedItem, _ := added["item"].(map[string]any)
	if got, _ := addedItem["id"].(string); !strings.HasPrefix(got, "fc_") {
		t.Fatalf("function_call item id = %q", got)
	}
	if addedItem["call_id"] != "call_abc" || addedItem["name"] != "get_weather" {
		t.Fatalf("function_call identity not mapped: %+v", addedItem)
	}
	if addedItem["status"] != "in_progress" {
		t.Fatalf("added item status = %v", addedItem["status"])
	}

	if got := strings.Join(argumentDeltas, ""); got != `{"city":"BJ"}` {
		t.Fatalf("argument deltas = %q, want %q", got, `{"city":"BJ"}`)
	}
	if argumentDone != `{"city":"BJ"}` {
		t.Fatalf("function_call_arguments.done = %q", argumentDone)
	}

	if doneItem == nil {
		t.Fatal("no completed function_call output_item.done")
	}
	if doneItem["status"] != "completed" || doneItem["arguments"] != `{"city":"BJ"}` {
		t.Fatalf("completed function_call item = %+v", doneItem)
	}

	if completedResponse == nil {
		t.Fatal("no response.completed event")
	}
	output, _ := completedResponse["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("response.completed output = %d items, want message + function_call: %+v", len(output), output)
	}
	var functionCall map[string]any
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] == "function_call" {
			functionCall = item
		}
	}
	if functionCall == nil {
		t.Fatalf("response.completed output lost the function_call: %+v", output)
	}
	if functionCall["call_id"] != "call_abc" || functionCall["name"] != "get_weather" || functionCall["arguments"] != `{"city":"BJ"}` {
		t.Fatalf("completed function_call payload = %+v", functionCall)
	}

	// Sequence numbers must be strictly increasing, as SDKs rely on them.
	previous := -1
	for _, ev := range events {
		if ev.Data == nil {
			continue
		}
		raw, ok := ev.Data["sequence_number"].(float64)
		if !ok {
			continue
		}
		if int(raw) <= previous {
			t.Fatalf("%s sequence_number %v is not increasing (previous %d)", ev.Event, raw, previous)
		}
		previous = int(raw)
	}
}

// TestResponsesEmitterTracksToolArgumentChars guards the fallback completion
// estimate: a tool-only stream has no assistant text, so the emitter must still
// account for the argument bytes it streamed.
func TestResponsesEmitterTracksToolArgumentChars(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	emitter := newResponsesStreamEmitter(c.Writer, w, "resp_test", "msg_test", "m")

	emitter.toolCallDelta(0, "call_1", "lookup", `{"q":`)
	emitter.toolCallDelta(0, "", "", `"hello"}`)

	if emitter.fullText.Len() != 0 {
		t.Fatalf("tool arguments must not be mixed into assistant text: %q", emitter.fullText.String())
	}
	want := len(`{"q":"hello"}`)
	if emitter.toolChars != want {
		t.Fatalf("toolChars = %d, want %d", emitter.toolChars, want)
	}
	if len(emitter.toolOrder) != 1 || emitter.toolStates[0].arguments.String() != `{"q":"hello"}` {
		t.Fatalf("tool state not accumulated: %+v", emitter.toolStates[0])
	}
}
