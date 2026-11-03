package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatRequestPreservesUnmodeledFields(t *testing.T) {
	body := `{"model":"alias","messages":[{"role":"user","content":"hi","cache_control":{"type":"ephemeral"}}],` +
		`"response_format":{"type":"json_schema","json_schema":{"name":"out","schema":{"type":"object"}}},` +
		`"seed":7,"logprobs":true,"top_logprobs":3,"parallel_tool_calls":false,` +
		`"reasoning_effort":"high","service_tier":"flex"}`

	var req ChatCompletionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	for _, key := range []string{
		"response_format", "seed", "logprobs", "top_logprobs",
		"reasoning_effort", "service_tier",
	} {
		if _, ok := req.Extra[key]; !ok {
			t.Fatalf("expected %q to be preserved, extra=%v", key, keysOf(req.Extra))
		}
	}
	if _, ok := req.Extra["model"]; ok {
		t.Fatal("modeled field 'model' must not be captured as an extra")
	}
	if _, ok := req.Extra["parallel_tool_calls"]; ok {
		t.Fatal("parallel_tool_calls is now a modeled field and must not be an extra")
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Fatalf("parallel_tool_calls not decoded into its field: %+v", req.ParallelToolCalls)
	}
	if _, ok := req.Messages[0].Extra["cache_control"]; !ok {
		t.Fatal("message-level unmodeled field was dropped")
	}

	// A gateway-side rewrite (model alias mapping, stream flag) must win over
	// anything the client sent.
	req.Model = "mapped"
	req.Stream = true
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var roundTrip map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("decode round trip: %v", err)
	}
	if string(roundTrip["model"]) != `"mapped"` {
		t.Fatalf("model overwritten by extra: %s", roundTrip["model"])
	}
	if string(roundTrip["stream"]) != `true` {
		t.Fatalf("stream overwritten by extra: %s", roundTrip["stream"])
	}
	if _, ok := roundTrip["response_format"]; !ok {
		t.Fatalf("response_format lost on encode: %s", encoded)
	}
	if _, ok := roundTrip["seed"]; !ok {
		t.Fatalf("seed lost on encode: %s", encoded)
	}
	if string(roundTrip["parallel_tool_calls"]) != `false` {
		t.Fatalf("parallel_tool_calls not forwarded from its field: %s", encoded)
	}
	if strings.Count(string(encoded), `"model"`) != 1 {
		t.Fatalf("duplicate model key emitted: %s", encoded)
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(roundTrip["messages"], &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if _, ok := messages[0]["cache_control"]; !ok {
		t.Fatalf("message-level extra lost: %s", roundTrip["messages"])
	}
}

func TestChatRequestWithoutExtrasIsUnchanged(t *testing.T) {
	var req ChatCompletionRequest
	if err := json.Unmarshal([]byte(`{"model":"m","temperature":0.5,"messages":[]}`), &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Extra) != 0 {
		t.Fatalf("unexpected extras: %v", keysOf(req.Extra))
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"m","messages":[],"temperature":0.5}`
	if string(encoded) != want {
		t.Fatalf("known-field wire form changed:\n got %s\nwant %s", encoded, want)
	}
}

func TestChatResponsePreservesUnmodeledFields(t *testing.T) {
	body := `{"id":"r1","object":"chat.completion","created":1,"model":"upstream",` +
		`"system_fingerprint":"fp_abc","service_tier":"scale",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":"hi","refusal":null},` +
		`"finish_reason":"stop","logprobs":{"content":[]}}],` +
		`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,` +
		`"completion_tokens_details":{"reasoning_tokens":5}}}`

	var resp ChatCompletionResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(resp.Extra["system_fingerprint"]) != `"fp_abc"` {
		t.Fatalf("system_fingerprint not preserved: %v", keysOf(resp.Extra))
	}
	if _, ok := resp.Choices[0].Extra["logprobs"]; !ok {
		t.Fatal("choice-level logprobs not preserved")
	}
	if _, ok := resp.Choices[0].Message.Extra["refusal"]; !ok {
		t.Fatal("message-level refusal not preserved")
	}
	if _, ok := resp.Usage.Extra["completion_tokens_details"]; !ok {
		t.Fatal("usage completion_tokens_details not preserved")
	}

	resp.Model = "alias"
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"system_fingerprint":"fp_abc"`, `"service_tier":"scale"`,
		`"logprobs"`, `"refusal":null`, `"completion_tokens_details"`, `"model":"alias"`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("encoded response lost %s: %s", field, encoded)
		}
	}
}

func TestChatChunkPreservesUnmodeledFields(t *testing.T) {
	body := `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"upstream",` +
		`"system_fingerprint":"fp_stream",` +
		`"choices":[{"index":0,"delta":{"role":"assistant","content":"he",` +
		`"refusal":null,"audio":{"id":"audio_1"}},` +
		`"logprobs":{"content":[{"token":"he"}]},"finish_reason":null}]}`

	var chunk ChatCompletionChunk
	if err := json.Unmarshal([]byte(body), &chunk); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	if string(chunk.Extra["system_fingerprint"]) != `"fp_stream"` {
		t.Fatal("chunk-level system_fingerprint not preserved")
	}
	if _, ok := chunk.Choices[0].Extra["logprobs"]; !ok {
		t.Fatal("chunk choice logprobs not preserved")
	}
	delta := chunk.Choices[0].Delta
	if _, ok := delta.Extra["refusal"]; !ok {
		t.Fatal("delta refusal not preserved")
	}
	if _, ok := delta.Extra["audio"]; !ok {
		t.Fatal("delta audio not preserved")
	}

	chunk.Model = "alias"
	encoded, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"system_fingerprint":"fp_stream"`, `"logprobs"`, `"refusal":null`, `"audio":{"id":"audio_1"}`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("encoded chunk lost %s: %s", field, encoded)
		}
	}
}

func TestChatChunkWithoutExtrasKeepsWireForm(t *testing.T) {
	chunk := ChatCompletionChunk{
		ID: "x", Object: "chat.completion.chunk", Created: 1, Model: "m",
		Choices: []ChunkChoice{},
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"x","object":"chat.completion.chunk","created":1,"model":"m","choices":[]}`
	if string(encoded) != want {
		t.Fatalf("unexpected wire form:\n got %s\nwant %s", encoded, want)
	}
}

func TestExtraFieldOrderIsDeterministic(t *testing.T) {
	var req ChatCompletionRequest
	_ = json.Unmarshal([]byte(`{"model":"m","seed":1,"response_format":{"type":"json_object"},"messages":[]}`), &req)
	first, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		next, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if string(next) != string(first) {
			t.Fatalf("non-deterministic encoding:\n%s\n%s", first, next)
		}
	}
}

func keysOf(extra ExtraFields) []string {
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	return keys
}
