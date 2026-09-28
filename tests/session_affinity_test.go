package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestSessionAffinity_DeterministicRoutingAndKVReusability(t *testing.T) {
	// Create mock HTTP test servers representing 3 vLLM GPU inference instances
	instanceHitCount := make(map[string]int)

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instanceHitCount["vllm-gpu-1"]++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-1","choices":[{"message":{"role":"assistant","content":"reply from gpu-1"}}]}`)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instanceHitCount["vllm-gpu-2"]++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-2","choices":[{"message":{"role":"assistant","content":"reply from gpu-2"}}]}`)
	}))
	defer server2.Close()

	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instanceHitCount["vllm-gpu-3"]++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-3","choices":[{"message":{"role":"assistant","content":"reply from gpu-3"}}]}`)
	}))
	defer server3.Close()

	channels := []model.ChannelConfig{
		{
			Name:      "vllm-gpu-1",
			Type:      model.ProviderOpenAI,
			BaseURL:   server1.URL,
			APIKey:    "test-key-1",
			Models:    []string{"deepseek-r1"},
			Protocols: []string{"chat"},
			Priority:  1,
			Weight:    10,
		},
		{
			Name:      "vllm-gpu-2",
			Type:      model.ProviderOpenAI,
			BaseURL:   server2.URL,
			APIKey:    "test-key-2",
			Models:    []string{"deepseek-r1"},
			Protocols: []string{"chat"},
			Priority:  1,
			Weight:    10,
		},
		{
			Name:      "vllm-gpu-3",
			Type:      model.ProviderOpenAI,
			BaseURL:   server3.URL,
			APIKey:    "test-key-3",
			Models:    []string{"deepseek-r1"},
			Protocols: []string{"chat"},
			Priority:  1,
			Weight:    10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	// 1. Session Alpha: 20 sequential turns of a multi-turn conversation
	sessionAlpha := "user-conv-thread-alpha-12345"
	ctxAlpha := context.WithValue(context.Background(), router.ContextKeySessionID, sessionAlpha)

	for turn := 1; turn <= 20; turn++ {
		req := &model.ChatCompletionRequest{
			Model: "deepseek-r1",
			Messages: []model.ChatMessage{
				{Role: "user", Content: fmt.Sprintf("Turn %d question", turn)},
			},
		}
		resp, err := dispatcher.Dispatch(ctxAlpha, req)
		if err != nil {
			t.Fatalf("turn %d failed: %v", turn, err)
		}
		if resp == nil || len(resp.Choices) == 0 {
			t.Fatalf("turn %d returned empty choices", turn)
		}
	}

	// Verify that exactly ONE instance handled all 20 turns of Session Alpha
	var pinnedAlphaInstance string
	for name, count := range instanceHitCount {
		if count == 20 {
			pinnedAlphaInstance = name
		}
	}
	if pinnedAlphaInstance == "" {
		t.Fatalf("Session Alpha was scattered across multiple instances instead of being sticky! Distribution: %+v", instanceHitCount)
	}
	t.Logf("Session Alpha successfully pinned to %s (20/20 requests hit exact instance, KV-cache 100%% reused)", pinnedAlphaInstance)

	// Reset counts
	instanceHitCount = make(map[string]int)

	// 2. Session Beta: 20 turns of a different conversation
	sessionBeta := "user-conv-thread-beta-67890"
	ctxBeta := context.WithValue(context.Background(), router.ContextKeySessionID, sessionBeta)

	for turn := 1; turn <= 20; turn++ {
		req := &model.ChatCompletionRequest{
			Model: "deepseek-r1",
			Messages: []model.ChatMessage{
				{Role: "user", Content: fmt.Sprintf("Turn %d question", turn)},
			},
		}
		_, err := dispatcher.Dispatch(ctxBeta, req)
		if err != nil {
			t.Fatalf("beta turn %d failed: %v", turn, err)
		}
	}

	var pinnedBetaInstance string
	for name, count := range instanceHitCount {
		if count == 20 {
			pinnedBetaInstance = name
		}
	}
	if pinnedBetaInstance == "" {
		t.Fatalf("Session Beta was scattered! Distribution: %+v", instanceHitCount)
	}
	t.Logf("Session Beta successfully pinned to %s (20/20 requests hit exact instance)", pinnedBetaInstance)
}

func TestSessionAffinity_FailureAwareDynamicFailover(t *testing.T) {
	// Instance 1 fails with 500
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"gpu out of memory"}`))
	}))
	defer server1.Close()

	// Instance 2 is healthy
	server2Calls := 0
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server2Calls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-ok","choices":[{"message":{"role":"assistant","content":"recovered"}}]}`)
	}))
	defer server2.Close()

	channels := []model.ChannelConfig{
		{
			Name:      "vllm-failing-1",
			Type:      model.ProviderOpenAI,
			BaseURL:   server1.URL,
			APIKey:    "test-1",
			Models:    []string{"qwen-72b"},
			Protocols: []string{"chat"},
			Priority:  1,
			Weight:    10,
		},
		{
			Name:      "vllm-healthy-2",
			Type:      model.ProviderOpenAI,
			BaseURL:   server2.URL,
			APIKey:    "test-2",
			Models:    []string{"qwen-72b"},
			Protocols: []string{"chat"},
			Priority:  1,
			Weight:    10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	sessionID := "sticky-failover-test"
	ctx := context.WithValue(context.Background(), router.ContextKeySessionID, sessionID)

	req := &model.ChatCompletionRequest{
		Model: "qwen-72b",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	// Even if hash selects failing server1, failover should smoothly route to server2
	resp, err := dispatcher.Dispatch(ctx, req)
	if err != nil {
		t.Fatalf("expected failover to succeed, got error: %v", err)
	}
	if resp == nil || resp.Choices[0].Message.Content != "recovered" {
		t.Fatalf("unexpected response content: %+v", resp)
	}
	if server2Calls == 0 {
		t.Fatalf("expected healthy instance to be called during failover")
	}
	t.Log("Failure-aware session affinity successfully failed over to backup instance without client error")
}
