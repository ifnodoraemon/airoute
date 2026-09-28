package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestCascadingAndUnrestrictedModelMapping(t *testing.T) {
	var receivedUpstreamModel string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req model.ChatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedUpstreamModel = req.Model

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "chatcmpl-cascade-test",
			"object": "chat.completion",
			"created": 1700000000,
			"model": "` + req.Model + `",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Cascading model success!"},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer mockServer.Close()

	channels := []model.ChannelConfig{
		// Channel 1: Wildcard prefix mapping "yy/*": "*" (e.g. yy/xxx/xx -> xxx/xx)
		{
			Name:    "cascade-channel",
			Type:    model.ProviderOpenAI,
			BaseURL: mockServer.URL,
			APIKey:  "test-key",
			Models:  []string{"xxx/xx", "deepseek-ai/DeepSeek-V3"},
			ModelMapping: map[string]string{
				"yy/*":          "*",
				"org/dept/v1/*": "*",
			},
			Priority: 1,
			Weight:   10,
		},
		// Channel 2: Automatic channel prefix stripping
		{
			Name:     "provider-beta",
			Type:     model.ProviderOpenAI,
			BaseURL:  mockServer.URL,
			APIKey:   "test-key",
			Models:   []string{"meta-llama/Llama-3.1-70B-Instruct"},
			Priority: 2,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	// Case 1: Cascading pattern yy/xxx/xx -> forwarded as xxx/xx
	req1 := &model.ChatCompletionRequest{
		Model: "yy/xxx/xx",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	resp1, err := dispatcher.Dispatch(context.Background(), req1)
	if err != nil {
		t.Fatalf("unexpected error for yy/xxx/xx: %v", err)
	}
	if resp1.Choices[0].Message.GetContentString() != "Cascading model success!" {
		t.Fatalf("unexpected response: %v", resp1)
	}
	if receivedUpstreamModel != "xxx/xx" {
		t.Fatalf("expected upstream to receive 'xxx/xx', got '%s'", receivedUpstreamModel)
	}

	// Case 2: Deeply nested multi-segment cascade: org/dept/v1/deepseek-ai/DeepSeek-V3
	req2 := &model.ChatCompletionRequest{
		Model: "org/dept/v1/deepseek-ai/DeepSeek-V3",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello deep nesting"},
		},
	}
	_, err = dispatcher.Dispatch(context.Background(), req2)
	if err != nil {
		t.Fatalf("unexpected error for deep nesting: %v", err)
	}
	if receivedUpstreamModel != "deepseek-ai/DeepSeek-V3" {
		t.Fatalf("expected upstream to receive 'deepseek-ai/DeepSeek-V3', got '%s'", receivedUpstreamModel)
	}

	// Case 3: Automatic channel name prefix: provider-beta/meta-llama/Llama-3.1-70B-Instruct
	req3 := &model.ChatCompletionRequest{
		Model: "provider-beta/meta-llama/Llama-3.1-70B-Instruct",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello auto-prefix"},
		},
	}
	_, err = dispatcher.Dispatch(context.Background(), req3)
	if err != nil {
		t.Fatalf("unexpected error for auto-prefix: %v", err)
	}
	if receivedUpstreamModel != "meta-llama/Llama-3.1-70B-Instruct" {
		t.Fatalf("expected upstream to receive 'meta-llama/Llama-3.1-70B-Instruct', got '%s'", receivedUpstreamModel)
	}

	// Case 4: Channel prefix with mapped alias: cascade-channel/yy/xxx/xx
	req4 := &model.ChatCompletionRequest{
		Model: "cascade-channel/yy/xxx/xx",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello prefix+alias"},
		},
	}
	_, err = dispatcher.Dispatch(context.Background(), req4)
	if err != nil {
		t.Fatalf("unexpected error for prefix+alias: %v", err)
	}
	if receivedUpstreamModel != "xxx/xx" {
		t.Fatalf("expected upstream to receive 'xxx/xx', got '%s'", receivedUpstreamModel)
	}
}
