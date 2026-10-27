package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// R-01: no provider call may be unbounded. The shared transport has no fixed
// header deadline anymore, so the channel timeout (or the documented default when a
// channel sets none) has to supply the deadline on every path.
func TestChannelCallContextAlwaysBounds(t *testing.T) {
	ctx, cancel := channelCallContext(context.Background(), &model.ChannelConfig{})
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("a channel without timeout_seconds left the upstream call unbounded (no deadline)")
	}
	remaining := time.Until(deadline)
	want := time.Duration(model.DefaultChannelTimeoutSeconds) * time.Second
	if remaining < want-5*time.Second || remaining > want+time.Second {
		t.Fatalf("default bound = %v, want %v", remaining, want)
	}

	configured, cancelConfigured := channelCallContext(context.Background(), &model.ChannelConfig{TimeoutSeconds: 300})
	defer cancelConfigured()
	deadline, ok = configured.Deadline()
	if !ok {
		t.Fatal("configured channel timeout produced no deadline")
	}
	if remaining := time.Until(deadline); remaining < 295*time.Second {
		t.Fatalf("bound = %v, want the configured 300s", remaining)
	}
}

func slowServer(t *testing.T, delay time.Duration, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// R-13: the multimodal dispatch path used to ignore timeout_seconds completely; a
// hung upstream held the request until the client left.
func TestDispatchHTTPHonorsChannelTimeout(t *testing.T) {
	srv := slowServer(t, 1500*time.Millisecond, `{"data":[]}`)
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "img", Type: model.ProviderOpenAI, BaseURL: srv.URL,
		Models: []string{"img-model"}, Protocols: []string{"images"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 1,
	}})

	start := time.Now()
	_, err := d.DispatchHTTP(context.Background(), &UpstreamRequest{
		Path: "/v1/images/generations", Method: http.MethodPost,
		Body: []byte(`{"model":"img-model","prompt":"x"}`), ContentType: "application/json",
		Model: "img-model", Protocol: "images",
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("dispatch ignored the 1s channel timeout and succeeded after %v", elapsed)
	}
	if elapsed > 1400*time.Millisecond {
		t.Fatalf("dispatch returned after %v; the 1s channel budget was not applied", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Logf("dispatch error (accepted, budget enforced): %v", err)
	}
}

// R-13: embeddings used to run without any channel budget as well.
func TestDispatchEmbeddingHonorsChannelTimeout(t *testing.T) {
	srv := slowServer(t, 1500*time.Millisecond, `{"data":[{"embedding":[0.1],"index":0}]}`)
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "emb", Type: model.ProviderOpenAI, BaseURL: srv.URL + "/v1",
		Models: []string{"emb-model"}, Protocols: []string{"embeddings"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 1,
	}})

	start := time.Now()
	_, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "emb-model", Input: "hi"})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("embedding dispatch ignored the 1s channel timeout and succeeded after %v", elapsed)
	}
	if elapsed > 1400*time.Millisecond {
		t.Fatalf("embedding dispatch returned after %v; the 1s channel budget was not applied", elapsed)
	}
}

// R-13: rerank shares the same omission.
func TestDispatchRerankHonorsChannelTimeout(t *testing.T) {
	srv := slowServer(t, 1500*time.Millisecond, `{"results":[{"index":0,"relevance_score":0.5}]}`)
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "rr", Type: model.ProviderOpenAI, BaseURL: srv.URL + "/v1",
		Models: []string{"rr-model"}, Protocols: []string{"rerank"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 1,
	}})

	start := time.Now()
	_, err := d.DispatchRerank(context.Background(), &model.RerankRequest{Model: "rr-model", Query: "q", Documents: []any{"d"}})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("rerank dispatch ignored the 1s channel timeout and succeeded after %v", elapsed)
	}
	if elapsed > 1400*time.Millisecond {
		t.Fatalf("rerank dispatch returned after %v; the 1s channel budget was not applied", elapsed)
	}
}

// scriptedStreamProvider emits a scripted prefix and then blocks until its context
// is cancelled, modelling an upstream that keeps the stream open.
type scriptedStreamProvider struct {
	events []*model.StreamEvent
	block  bool
}

func (p *scriptedStreamProvider) Name() string             { return "scripted-stream" }
func (p *scriptedStreamProvider) Type() model.ProviderType { return model.ProviderCustom }
func (p *scriptedStreamProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("not used")
}
func (p *scriptedStreamProvider) ChatCompleteStream(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	out := make(chan *model.StreamEvent, len(p.events)+1)
	for _, ev := range p.events {
		out <- ev
	}
	if p.block {
		go func() {
			defer close(out)
			<-ctx.Done()
		}()
		return out, nil
	}
	close(out)
	return out, nil
}

// R-02 (dispatcher side): the provider stream is bounded by the channel timeout,
// not by an absolute 120s handler deadline and not left unbounded.
func TestStreamAttemptBoundedByChannelTimeout(t *testing.T) {
	provider_ := &scriptedStreamProvider{
		events: []*model.StreamEvent{{Chunk: &model.ChatCompletionChunk{Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{Content: "first"}}}}}},
		block:  true,
	}
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "stream", Type: model.ProviderCustom, Models: []string{"m"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 1,
	}})
	d.RegisterProvider(provider_)

	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true})
	if err != nil {
		t.Fatalf("dispatch stream: %v", err)
	}
	closed := make(chan struct{})
	go func() {
		for range stream {
		}
		close(closed)
	}()
	select {
	case <-closed:
		// The channel budget ended the stream.
	case <-time.After(2500 * time.Millisecond):
		t.Fatalf("provider stream ran past the 1s channel timeout: the attempt is not bounded by timeout_seconds")
	}
}
