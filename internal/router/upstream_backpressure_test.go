package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func TestCooldownSkipsLaterRequestsWithoutPoisoningOtherModels(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"slow down"}}`)
	}))
	defer server.Close()
	ch := model.ChannelConfig{Name: "a", Type: model.ProviderOpenAI, BaseURL: server.URL, APIKey: "k", Models: []string{"m", "other"}}
	d := NewDispatcher([]model.ChannelConfig{ch})
	req := &model.ChatCompletionRequest{Model: "m"}
	for i := 0; i < 2; i++ {
		_, err := d.Dispatch(context.Background(), req)
		ue, ok := provider.AsUpstreamError(err)
		if !ok || ue.Status != 429 {
			t.Fatalf("expected retryable 429, got %v", err)
		}
		if provider.RetryDelay(ue.Headers, time.Now()) < 25*time.Second {
			t.Fatal("Retry-After was lost")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("cooled upstream called %d times", calls.Load())
	}
	if err := d.cooldowns.check(&ch, "other"); err != nil {
		t.Fatal("model-local 429 cooled an unrelated model")
	}
	if d.GetBreakerStatus("a") != "CLOSED" {
		t.Fatal("429 opened the breaker")
	}
	d.cooldowns.mu.Lock()
	d.cooldowns.until[cooldownKey(&ch, "m")] = time.Now().Add(-time.Second)
	d.cooldowns.mu.Unlock()
	_, _ = d.Dispatch(context.Background(), req)
	if calls.Load() != 2 {
		t.Fatal("expired cooldown did not recover")
	}
}

func TestConcurrencyQueueCancellationAndSharedCredential(t *testing.T) {
	ch := model.ChannelConfig{Name: "a", BaseURL: "https://unit.invalid", APIKey: "k", MaxConcurrency: 1, QueueLimit: 1, QueueWaitSeconds: 2}
	lanes := newCallLanes([]model.ChannelConfig{ch})
	release, err := lanes.acquire(context.Background(), &ch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		r, e := lanes.acquire(ctx, &ch)
		if e == nil {
			r()
		}
		finished <- e
	}()
	deadline := time.Now().Add(time.Second)
	for {
		lanes.mu.Lock()
		n := len(lanes.lanes[upstreamIdentity(&ch)].waiters)
		lanes.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request did not queue")
		}
		time.Sleep(time.Millisecond)
	}
	copy := ch
	copy.Name = "same-credential"
	if r, e := lanes.acquire(context.Background(), &copy); e == nil {
		r()
		t.Fatal("duplicate channel escaped the credential's queue limit")
	}
	cancel()
	if e := <-finished; e != context.Canceled {
		t.Fatalf("queued cancellation=%v", e)
	}
	release()
	release()
	r, e := lanes.acquire(context.Background(), &ch)
	if e != nil {
		t.Fatalf("slot leaked after cancellation: %v", e)
	}
	r()
}

func TestHTTPConcurrencySlotLastsUntilBodyClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":true}`) }))
	defer server.Close()
	d := NewDispatcher([]model.ChannelConfig{{Name: "a", Type: model.ProviderOpenAI, BaseURL: server.URL, Models: []string{"m"}, MaxConcurrency: 1}})
	req := &UpstreamRequest{Model: "m", Path: "/v1/images/generations", Protocol: "images", Method: "POST", ContentType: "application/json", Body: []byte(`{"model":"m"}`)}
	first, e := d.DispatchHTTP(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := d.DispatchHTTP(context.Background(), req); e == nil {
		second.Stream.Close()
		t.Fatal("slot released before the body was closed")
	}
	_, _ = io.Copy(io.Discard, first.Stream)
	first.Stream.Close()
	third, e := d.DispatchHTTP(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	third.Stream.Close()
	if strings.Contains(d.GetBreakerStatus("a"), "OPEN") {
		t.Fatal("local capacity rejection affected the breaker")
	}
}
