package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression tests for BUG-GEMINI-TOOLCALL (fixed).
//
// OpenAI streams a tool call as index-keyed fragments (the first fragment carries
// the id and name, later ones carry slices of the JSON arguments string). A
// Gemini `functionCall` part must instead carry the complete name and a complete
// args object. The old translation emitted one functionCall per fragment, so a
// single upstream call reached the Gemini client as three parts — the first with
// a name and args=null, the rest with an empty name and args=null — and the
// assembled arguments never arrived at all.
//
// These tests drive the real handler (router + httptest upstream) and assert the
// Gemini-visible SSE frames.

const geminiToolProbeModel = "gemini-tool-model"

// runGeminiStreamProbe drives handleGeminiStream against an OpenAI-compatible
// upstream and returns the Gemini SSE body the client would receive.
func runGeminiStreamProbe(t *testing.T, sseBody string) string {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseBody)
	}))
	defer upstream.Close()

	// The audit/settlement path needs a pricing engine even though these tests
	// only inspect the SSE shape.
	_ = edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: geminiToolProbeModel, GroupName: "default",
		PromptPrice: 1, CompletionPrice: 2, Currency: "CNY",
	})
	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "openai-for-gemini", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{geminiToolProbeModel}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)

	canonical := &model.ChatCompletionRequest{
		Model:    geminiToolProbeModel,
		Stream:   true,
		Messages: []model.ChatMessage{{Role: "user", Content: "weather in Paris?"}},
	}
	c, w := newTestContext(http.MethodPost, "/v1beta/models/"+geminiToolProbeModel+":streamGenerateContent")
	handler.handleGeminiStream(c, c.Request.Context(), canonical, geminiToolProbeModel, "sess-gemini-tool", time.Now())
	return w.Body.String()
}

// geminiSSEDataFrames extracts the JSON payloads of every `data:` frame in an
// SSE body (the cost trailer comment is skipped).
func geminiSSEDataFrames(t *testing.T, body string) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var frame map[string]any
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			t.Fatalf("data frame is not JSON: %v (%q)", err, payload)
		}
		frames = append(frames, frame)
	}
	return frames
}

// geminiCall is one functionCall as seen by a Gemini client.
type geminiCall struct {
	Name string
	Args map[string]any
}

// geminiFunctionCalls collects every functionCall part across all frames, in
// emission order.
func geminiFunctionCalls(t *testing.T, frames []map[string]any) []geminiCall {
	t.Helper()
	var calls []geminiCall
	for _, frame := range frames {
		candidates, _ := frame["candidates"].([]any)
		for _, rawCandidate := range candidates {
			candidate, _ := rawCandidate.(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				rawCall, ok := part["functionCall"].(map[string]any)
				if !ok {
					continue
				}
				name, _ := rawCall["name"].(string)
				args, _ := rawCall["args"].(map[string]any)
				calls = append(calls, geminiCall{Name: name, Args: args})
			}
		}
	}
	return calls
}

// geminiStreamedText concatenates every text part across all frames.
func geminiStreamedText(frames []map[string]any) string {
	var text strings.Builder
	for _, frame := range frames {
		candidates, _ := frame["candidates"].([]any)
		for _, rawCandidate := range candidates {
			candidate, _ := rawCandidate.(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if textPart, ok := part["text"].(string); ok {
					text.WriteString(textPart)
				}
			}
		}
	}
	return text.String()
}

// TestGeminiStreamAssemblesToolCallFragments is the original BUG-GEMINI-TOOLCALL
// reproduction, now asserting the fixed behaviour: one upstream tool call split
// across three argument fragments becomes exactly one complete functionCall.
func TestGeminiStreamAssemblesToolCallFragments(t *testing.T) {
	body := runGeminiStreamProbe(t, `data: {"id":"s1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`)

	frames := geminiSSEDataFrames(t, body)
	calls := geminiFunctionCalls(t, frames)
	if len(calls) != 1 {
		t.Fatalf("expected exactly one functionCall part (fragments must be assembled), got %d: %+v\nSSE:\n%s", len(calls), calls, body)
	}
	if calls[0].Name != "get_weather" {
		t.Fatalf("functionCall name = %q, want %q", calls[0].Name, "get_weather")
	}
	if got, want := calls[0].Args["city"], "Paris"; got != want {
		t.Fatalf("functionCall args.city = %v, want %v (arguments were not assembled)", got, want)
	}

	// The assembled call must ride the terminal frame, together with the finish
	// reason, so a Gemini client sees a well-formed terminal candidate.
	last := frames[len(frames)-1]
	candidate := last["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "STOP" {
		t.Fatalf("terminal frame finishReason = %v, want STOP", candidate["finishReason"])
	}
}

// TestGeminiStreamAssemblesParallelToolCalls covers two parallel calls whose
// argument fragments are interleaved (index 0 and index 1).
func TestGeminiStreamAssemblesParallelToolCalls(t *testing.T) {
	body := runGeminiStreamProbe(t, `data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","function":{"name":"get_time","arguments":""}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"zone\":\"CET\"}"}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`)

	calls := geminiFunctionCalls(t, geminiSSEDataFrames(t, body))
	if len(calls) != 2 {
		t.Fatalf("expected two parallel functionCall parts, got %d: %+v\nSSE:\n%s", len(calls), calls, body)
	}
	if calls[0].Name != "get_weather" || calls[0].Args["city"] != "Paris" {
		t.Fatalf("first call = %+v, want get_weather{city:Paris}", calls[0])
	}
	if calls[1].Name != "get_time" || calls[1].Args["zone"] != "CET" {
		t.Fatalf("second call = %+v, want get_time{zone:CET}", calls[1])
	}
}

// TestGeminiStreamTextAndToolInterleaved covers text deltas emitted before and
// after tool-call fragments: text stays real-time, and the call is emitted once.
func TestGeminiStreamTextAndToolInterleaved(t *testing.T) {
	body := runGeminiStreamProbe(t, `data: {"id":"s1","choices":[{"index":0,"delta":{"role":"assistant","content":"Checking "},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"get_weather","arguments":"{\"city\":"}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"content":"now."},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`)

	frames := geminiSSEDataFrames(t, body)
	if got, want := geminiStreamedText(frames), "Checking now."; got != want {
		t.Fatalf("streamed text = %q, want %q (text must keep flowing in real time)", got, want)
	}

	calls := geminiFunctionCalls(t, frames)
	if len(calls) != 1 {
		t.Fatalf("expected exactly one functionCall, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "get_weather" || calls[0].Args["city"] != "Paris" {
		t.Fatalf("assembled call = %+v, want get_weather{city:Paris}", calls[0])
	}
}

// TestGeminiStreamAssemblesToolCallWithoutIndex covers upstreams that omit
// tool_call.index: fragments are ordered by appearance (the fragment carrying the
// name starts a call, bare argument fragments continue it).
func TestGeminiStreamAssemblesToolCallWithoutIndex(t *testing.T) {
	body := runGeminiStreamProbe(t, `data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","function":{"name":"lookup","arguments":"{\"q\":"}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"function":{"arguments":"\"abc\"}"}}]},"finish_reason":null}]}

data: {"id":"s1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`)

	calls := geminiFunctionCalls(t, geminiSSEDataFrames(t, body))
	if len(calls) != 1 {
		t.Fatalf("expected one assembled call for index-less fragments, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "lookup" || calls[0].Args["q"] != "abc" {
		t.Fatalf("assembled call = %+v, want lookup{q:abc}", calls[0])
	}
}

// TestGeminiStreamFlushesToolCallWithoutTerminalFrame covers a stream that ends
// (or sends [DONE]) without a finish_reason frame: the assembled call must still
// reach the client exactly once instead of being dropped.
func TestGeminiStreamFlushesToolCallWithoutTerminalFrame(t *testing.T) {
	body := runGeminiStreamProbe(t, `data: {"id":"s1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":null}]}

data: [DONE]

`)

	calls := geminiFunctionCalls(t, geminiSSEDataFrames(t, body))
	if len(calls) != 1 {
		t.Fatalf("expected the call to be flushed at stream end, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "get_weather" || calls[0].Args["city"] != "Paris" {
		t.Fatalf("flushed call = %+v, want get_weather{city:Paris}", calls[0])
	}
}
