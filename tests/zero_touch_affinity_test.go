package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/api"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
)

// TestZeroTouchSessionAffinity verifies that multi-turn conversations automatically stick to the
// same upstream provider without requiring the client to pass ANY custom session headers.
func TestZeroTouchSessionAffinity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var nodeAHits int64
	var nodeBHits int64

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&nodeAHits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-A","choices":[{"message":{"role":"assistant","content":"Response from Upstream Node A"}}]}`)
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&nodeBHits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-B","choices":[{"message":{"role":"assistant","content":"Response from Upstream Node B"}}]}`)
	}))
	defer serverB.Close()

	channels := []model.ChannelConfig{
		{
			ID:       1,
			Name:     "upstream-node-A",
			BaseURL:  serverA.URL,
			Type:     model.ProviderOpenAI,
			Priority: 1,
			Weight:   50,
			Models:   []string{"deepseek-chat"},
		},
		{
			ID:       2,
			Name:     "upstream-node-B",
			BaseURL:  serverB.URL,
			Type:     model.ProviderOpenAI,
			Priority: 1,
			Weight:   50,
			Models:   []string{"deepseek-chat"},
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)
	r := gin.New()
	r.POST("/v1/chat/completions", handler.HandleChatCompletions)

	// 1. Turn 1: Client sends initial user question (WITHOUT ANY X-Session-ID or Cookie)
	turn1Body := model.ChatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []model.ChatMessage{
			{Role: "system", Content: "You are a coding assistant"},
			{Role: "user", Content: "Write a high-performance LRU cache in Go"},
		},
	}
	turn1Bytes, _ := json.Marshal(turn1Body)

	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(turn1Bytes))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()

	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("Turn 1 failed: status %d, body %s", w1.Code, w1.Body.String())
	}

	// Gateway must output X-Nano-Session-ID and Set-Cookie automatically
	sessionID := w1.Header().Get("X-Nano-Session-ID")
	if sessionID == "" {
		t.Fatalf("Expected X-Nano-Session-ID in response header, got empty")
	}
	t.Logf("Turn 1 automatically generated zero-touch Session ID: %s", sessionID)

	cookies := w1.Result().Cookies()
	var nanoCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "nano_session" {
			nanoCookie = c
			break
		}
	}
	if nanoCookie == nil {
		t.Fatalf("Expected nano_session cookie to be set, but was not found")
	}

	totalTurn1A := atomic.LoadInt64(&nodeAHits)
	totalTurn1B := atomic.LoadInt64(&nodeBHits)
	var expectedPinnedNode string
	if totalTurn1A == 1 && totalTurn1B == 0 {
		expectedPinnedNode = "A"
	} else if totalTurn1B == 1 && totalTurn1A == 0 {
		expectedPinnedNode = "B"
	} else {
		t.Fatalf("Expected exactly 1 hit on either node A or B, got A=%d, B=%d", totalTurn1A, totalTurn1B)
	}
	t.Logf("Turn 1 was routed to Upstream Node %s", expectedPinnedNode)

	// 2. Turn 2: Client continues conversation. NO HEADERS, NO COOKIES passed!
	// (Simulating a bare-bones Python/curl client that doesn't save cookies)
	turn2Body := model.ChatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []model.ChatMessage{
			{Role: "system", Content: "You are a coding assistant"},
			{Role: "user", Content: "Write a high-performance LRU cache in Go"},
			{Role: "assistant", Content: "Here is an LRU cache implementation..."},
			{Role: "user", Content: "Now add thread-safe mutex and TTL expiration support"},
		},
	}
	turn2Bytes, _ := json.Marshal(turn2Body)

	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(turn2Bytes))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Turn 2 failed: status %d, body %s", w2.Code, w2.Body.String())
	}

	sessionID2 := w2.Header().Get("X-Nano-Session-ID")
	if sessionID2 != sessionID {
		t.Fatalf("Expected Turn 2 to derive identical session ID %s, but got %s", sessionID, sessionID2)
	}
	t.Logf("Turn 2 zero-touch derived identical Session ID: %s", sessionID2)

	// Check that Turn 2 went to the EXACT same node as Turn 1
	totalTurn2A := atomic.LoadInt64(&nodeAHits)
	totalTurn2B := atomic.LoadInt64(&nodeBHits)
	if expectedPinnedNode == "A" {
		if totalTurn2A != 2 || totalTurn2B != 0 {
			t.Fatalf("Turn 2 violated affinity! Expected Node A to have 2 hits and B to have 0, got A=%d, B=%d", totalTurn2A, totalTurn2B)
		}
	} else {
		if totalTurn2B != 2 || totalTurn2A != 0 {
			t.Fatalf("Turn 2 violated affinity! Expected Node B to have 2 hits and A to have 0, got A=%d, B=%d", totalTurn2A, totalTurn2B)
		}
	}

	// 3. Turn 3: Also test with Cookie passed
	turn3Body := model.ChatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Third turn with cookie"},
		},
	}
	turn3Bytes, _ := json.Marshal(turn3Body)
	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(turn3Bytes))
	req3.Header.Set("Content-Type", "application/json")
	req3.AddCookie(nanoCookie)
	w3 := httptest.NewRecorder()

	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("Turn 3 failed: status %d", w3.Code)
	}

	// Must also route to the same pinned node
	totalTurn3A := atomic.LoadInt64(&nodeAHits)
	totalTurn3B := atomic.LoadInt64(&nodeBHits)
	if expectedPinnedNode == "A" {
		if totalTurn3A != 3 || totalTurn3B != 0 {
			t.Fatalf("Turn 3 with cookie violated affinity! A=%d, B=%d", totalTurn3A, totalTurn3B)
		}
	} else {
		if totalTurn3B != 3 || totalTurn3A != 0 {
			t.Fatalf("Turn 3 with cookie violated affinity! A=%d, B=%d", totalTurn3A, totalTurn3B)
		}
	}
	t.Logf("100%% Verified: All 3 turns successfully routed to Node %s with Zero-Touch Session Affinity!", expectedPinnedNode)
}
