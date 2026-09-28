package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestSmoothWeightedRoundRobin_EqualPriorityNoStarvation(t *testing.T) {
	var countA int64
	var countB int64

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countA, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-a","choices":[{"message":{"role":"assistant","content":"from A"}}]}`)
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countB, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-b","choices":[{"message":{"role":"assistant","content":"from B"}}]}`)
	}))
	defer serverB.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "ch-a",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverA.URL,
			Models:   []string{"gpt-test"},
			Priority: 1,
			Weight:   10,
		},
		{
			Name:     "ch-b",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverB.URL,
			Models:   []string{"gpt-test"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	// Send 100 requests - both should receive ~50 requests, zero starvation!
	totalRequests := 100
	for i := 0; i < totalRequests; i++ {
		resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
			Model: "gpt-test",
			Messages: []model.ChatMessage{
				{Role: "user", Content: "hello"},
			},
		})
		if err != nil {
			t.Fatalf("dispatch error: %v", err)
		}
		if resp == nil {
			t.Fatalf("nil response")
		}
	}

	a := atomic.LoadInt64(&countA)
	b := atomic.LoadInt64(&countB)

	t.Logf("Equal Priority Dispatch Results: A=%d, B=%d", a, b)

	if a == 0 || b == 0 {
		t.Fatalf("Channel starvation detected! A=%d, B=%d", a, b)
	}
	if a != 50 || b != 50 {
		t.Fatalf("Expected exactly 50/50 smooth distribution, got A=%d, B=%d", a, b)
	}
}

func TestSmoothWeightedRoundRobin_WeightedDistribution(t *testing.T) {
	var countA int64
	var countB int64

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countA, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-a","choices":[{"message":{"role":"assistant","content":"from A"}}]}`)
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countB, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-b","choices":[{"message":{"role":"assistant","content":"from B"}}]}`)
	}))
	defer serverB.Close()

	// Weight 20 vs 10 (2:1 ratio)
	channels := []model.ChannelConfig{
		{
			Name:     "ch-heavy",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverA.URL,
			Models:   []string{"gpt-test-w"},
			Priority: 1,
			Weight:   20,
		},
		{
			Name:     "ch-light",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverB.URL,
			Models:   []string{"gpt-test-w"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	for i := 0; i < 90; i++ {
		_, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
			Model: "gpt-test-w",
			Messages: []model.ChatMessage{
				{Role: "user", Content: "hello"},
			},
		})
		if err != nil {
			t.Fatalf("dispatch error: %v", err)
		}
	}

	a := atomic.LoadInt64(&countA)
	b := atomic.LoadInt64(&countB)

	t.Logf("Weighted Dispatch Results (2:1): A=%d, B=%d", a, b)

	if a != 60 || b != 30 {
		t.Fatalf("Expected 60 for A and 30 for B (2:1 ratio), got A=%d, B=%d", a, b)
	}
}

func TestMultiTierFallback_SameTierFirstThenBackup(t *testing.T) {
	var countPrimaryA int64
	var countPrimaryB int64
	var countBackup int64

	// Primary A fails
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countPrimaryA, 1)
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer serverA.Close()

	// Primary B succeeds
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countPrimaryB, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-b","choices":[{"message":{"role":"assistant","content":"from Primary B"}}]}`)
	}))
	defer serverB.Close()

	// Backup (Priority 2) should NOT be called since Primary B in Priority 1 succeeded
	serverBackup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countBackup, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-backup","choices":[{"message":{"role":"assistant","content":"from Backup"}}]}`)
	}))
	defer serverBackup.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "pri-a",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverA.URL,
			Models:   []string{"gpt-tier"},
			Priority: 1,
			Weight:   10,
		},
		{
			Name:     "pri-b",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverB.URL,
			Models:   []string{"gpt-tier"},
			Priority: 1,
			Weight:   10,
		},
		{
			Name:     "backup-c",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverBackup.URL,
			Models:   []string{"gpt-tier"},
			Priority: 2,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	for i := 0; i < 10; i++ {
		resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
			Model: "gpt-tier",
			Messages: []model.ChatMessage{
				{Role: "user", Content: "ping"},
			},
		})
		if err != nil {
			t.Fatalf("dispatch error: %v", err)
		}
		if resp == nil || len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "from Primary B" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	}

	backupCalled := atomic.LoadInt64(&countBackup)
	if backupCalled != 0 {
		t.Fatalf("Backup channel was prematurely called! Count: %d", backupCalled)
	}
}

func TestCircuitBreaker_BypassesDeadUpstream(t *testing.T) {
	var countA int64
	var countB int64

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countA, 1)
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&countB, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp-b","choices":[{"message":{"role":"assistant","content":"healthy B"}}]}`)
	}))
	defer serverB.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "node-a",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverA.URL,
			Models:   []string{"test-cb"},
			Priority: 1,
			Weight:   10,
		},
		{
			Name:     "node-b",
			Type:     model.ProviderOpenAI,
			BaseURL:  serverB.URL,
			Models:   []string{"test-cb"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	for i := 0; i < 20; i++ {
		_, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
			Model: "test-cb",
			Messages: []model.ChatMessage{
				{Role: "user", Content: "ping"},
			},
		})
		if err != nil {
			t.Fatalf("dispatch error: %v", err)
		}
	}

	statusA := dispatcher.GetBreakerStatus("node-a")
	if statusA != "OPEN" {
		t.Fatalf("expected node-a breaker to be OPEN after failures, got %s", statusA)
	}

	// Verify node-a is skipped without hitting serverA
	hitsBefore := atomic.LoadInt64(&countA)
	_, _ = dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
		Model: "test-cb",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "ping"},
		},
	})
	hitsAfter := atomic.LoadInt64(&countA)
	if hitsAfter != hitsBefore {
		t.Fatalf("dead upstream node-a was not bypassed while breaker is OPEN!")
	}
}
