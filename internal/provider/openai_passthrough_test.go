package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestOpenAIPassthroughPreservesRequestAndResponseFields(t *testing.T) {
	var received map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
		}
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"upstream",`+
			`"system_fingerprint":"fp_abc","service_tier":"scale",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok","refusal":null},`+
			`"finish_reason":"stop","logprobs":{"content":[]}}],`+
			`"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,`+
			`"completion_tokens_details":{"reasoning_tokens":2}}}`)
	}))
	defer server.Close()

	var req model.ChatCompletionRequest
	rawReq := `{"model":"alias","messages":[{"role":"user","content":"hi"}],` +
		`"response_format":{"type":"json_object"},"seed":42,` +
		`"reasoning_effort":"high","parallel_tool_calls":false}`
	if err := json.Unmarshal([]byte(rawReq), &req); err != nil {
		t.Fatal(err)
	}

	channel := &model.ChannelConfig{Name: "upstream", Type: model.ProviderOpenAI, BaseURL: server.URL}
	resp, err := NewOpenAIProvider(server.Client()).ChatComplete(context.Background(), &req, channel)
	if err != nil {
		t.Fatalf("chat complete: %v", err)
	}

	for _, key := range []string{"response_format", "seed", "reasoning_effort", "parallel_tool_calls"} {
		if _, ok := received[key]; !ok {
			t.Fatalf("upstream request lost %q: %v", key, receivedKeys(received))
		}
	}
	if string(received["seed"]) != "42" {
		t.Fatalf("seed rewritten: %s", received["seed"])
	}
	if string(received["model"]) != `"alias"` {
		t.Fatalf("model not forwarded: %s", received["model"])
	}

	if string(resp.Extra["system_fingerprint"]) != `"fp_abc"` {
		t.Fatalf("response lost system_fingerprint: %v", receivedKeys(resp.Extra))
	}
	if _, ok := resp.Choices[0].Extra["logprobs"]; !ok {
		t.Fatal("response choice lost logprobs")
	}
	if _, ok := resp.Choices[0].Message.Extra["refusal"]; !ok {
		t.Fatal("response message lost refusal")
	}
	if _, ok := resp.Usage.Extra["completion_tokens_details"]; !ok {
		t.Fatal("response usage lost completion_tokens_details")
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"system_fingerprint":"fp_abc"`, `"logprobs"`, `"refusal":null`,
		`"completion_tokens_details"`, `"model":"alias"`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("client-bound response lost %s: %s", field, encoded)
		}
	}
}

func TestOpenAIStreamPassthroughPreservesChunkFields(t *testing.T) {
	var received map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
		}
		_ = json.Unmarshal(raw, &received)

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"upstream\",\"system_fingerprint\":\"fp_stream\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"he\",\"refusal\":null,\"audio\":{\"id\":\"audio_1\"}},\"logprobs\":{\"content\":[{\"token\":\"he\"}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"upstream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"llo\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var req model.ChatCompletionRequest
	rawReq := `{"model":"alias","messages":[{"role":"user","content":"hi"}],` +
		`"response_format":{"type":"json_object"},"seed":7}`
	if err := json.Unmarshal([]byte(rawReq), &req); err != nil {
		t.Fatal(err)
	}

	channel := &model.ChannelConfig{Name: "upstream", Type: model.ProviderOpenAI, BaseURL: server.URL}
	events, err := NewOpenAIProvider(server.Client()).ChatCompleteStream(context.Background(), &req, channel)
	if err != nil {
		t.Fatalf("chat complete stream: %v", err)
	}

	var gotContent strings.Builder
	var firstChunk *model.ChatCompletionChunk
	for event := range events {
		if event.Err != nil {
			t.Fatalf("stream event error: %v", event.Err)
		}
		if event.Chunk == nil {
			continue
		}
		if firstChunk == nil && len(event.Chunk.Choices) > 0 && event.Chunk.Choices[0].Delta.Content != "" {
			firstChunk = event.Chunk
		}
		for _, choice := range event.Chunk.Choices {
			gotContent.WriteString(choice.Delta.Content)
		}
	}
	if firstChunk == nil {
		t.Fatal("no content chunk observed")
	}
	if gotContent.String() != "hello" {
		t.Fatalf("stream content mangled: %q", gotContent.String())
	}
	if string(firstChunk.Extra["system_fingerprint"]) != `"fp_stream"` {
		t.Fatalf("chunk lost system_fingerprint: %v", receivedKeys(firstChunk.Extra))
	}
	if _, ok := firstChunk.Choices[0].Extra["logprobs"]; !ok {
		t.Fatal("chunk choice lost logprobs")
	}
	if _, ok := firstChunk.Choices[0].Delta.Extra["refusal"]; !ok {
		t.Fatal("delta lost refusal")
	}
	if _, ok := firstChunk.Choices[0].Delta.Extra["audio"]; !ok {
		t.Fatal("delta lost audio")
	}

	encoded, err := json.Marshal(firstChunk)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"system_fingerprint":"fp_stream"`, `"logprobs"`, `"refusal":null`, `"audio":{"id":"audio_1"}`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("client-bound chunk lost %s: %s", field, encoded)
		}
	}
	for _, key := range []string{"response_format", "seed"} {
		if _, ok := received[key]; !ok {
			t.Fatalf("upstream stream request lost %q: %v", key, receivedKeys(received))
		}
	}
}

func receivedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
