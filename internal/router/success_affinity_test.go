package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestAffinityFollowsSuccessfulFallbackWithoutRedis(t *testing.T) {
	var rejected atomic.Int32
	var failingKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+failingKey {
			rejected.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"id":"ok","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer server.Close()
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "a", Type: model.ProviderOpenAI, BaseURL: server.URL, APIKey: "key-a", Models: []string{"m"}, Weight: 1},
		{Name: "b", Type: model.ProviderOpenAI, BaseURL: server.URL, APIKey: "key-b", Models: []string{"m"}, Weight: 1},
	})
	ctx := context.WithValue(context.Background(), ContextKeySessionID, "tenant-a/session")
	failingKey = d.GetChannelsForModelAndProtocolWithContext(ctx, "m", "chat")[0].APIKey
	req := &model.ChatCompletionRequest{Model: "m", Messages: []model.ChatMessage{{Role: "user", Content: "hi"}}}
	first, err := d.Dispatch(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Load() != 1 {
		t.Fatal("primary wasn't attempted once")
	}
	second, err := d.Dispatch(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Load() != 1 || second.Channel == "" || first.Channel == second.Channel {
		t.Fatalf("failed channel was retried instead of successful fallback: first=%s second=%s rejects=%d", first.Channel, second.Channel, rejected.Load())
	}
	if pin := d.sessionPins.get("m", "tenant-b/session"); pin != "" {
		t.Fatal("session pin crossed caller scope")
	}
}

func TestSessionPinsExpireAndStayBounded(t *testing.T) {
	p := newSessionPins()
	now := time.Now()
	p.now = func() time.Time { return now }
	for i := 0; i < maxSessionPins+1; i++ {
		p.put("m", fmt.Sprint(i), "a")
	}
	if len(p.entries) != maxSessionPins || p.get("m", "0") != "" {
		t.Fatal("cache did not evict its oldest entry")
	}
	p.put("m", "current", "b")
	p.forget("m", "current", "a")
	if p.get("m", "current") != "b" {
		t.Fatal("late failure deleted a newer successful pin")
	}
	now = now.Add(p.ttl)
	if p.get("m", "current") != "" {
		t.Fatal("expired pin remained usable")
	}
}
