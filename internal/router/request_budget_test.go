package router

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// blockingProvider models an upstream that hangs until the context is
// cancelled. It makes the budget observable: without one, Dispatch would wait
// for the full channel timeout.
type blockingProvider struct{}

func (blockingProvider) Name() string             { return "blocking" }
func (blockingProvider) Type() model.ProviderType { return model.ProviderCustom }
func (blockingProvider) ChatComplete(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingProvider) ChatCompleteStream(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestDispatchStopsAtRequestBudget(t *testing.T) {
	t.Setenv(RequestBudgetEnv, "1")

	dispatcher := NewDispatcher([]model.ChannelConfig{{
		ID: 1, Name: "slow", Type: model.ProviderCustom,
		BaseURL: "http://127.0.0.1:1", Models: []string{"slow-model"}, TimeoutSeconds: 30,
	}})
	dispatcher.RegisterProvider(blockingProvider{})

	start := time.Now()
	resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{Model: "slow-model"})
	elapsed := time.Since(start)

	if resp != nil {
		t.Fatalf("expected no response, got %+v", resp)
	}
	if !errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatalf("expected ErrRequestBudgetExceeded, got %v", err)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("dispatch returned before the budget elapsed: %v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("dispatch ignored the 1s budget: %v", elapsed)
	}
}

func TestDispatchRespectsCallerShorterDeadline(t *testing.T) {
	t.Setenv(RequestBudgetEnv, "30")

	dispatcher := NewDispatcher([]model.ChannelConfig{{
		ID: 1, Name: "slow", Type: model.ProviderCustom,
		BaseURL: "http://127.0.0.1:1", Models: []string{"slow-model"}, TimeoutSeconds: 30,
	}})
	dispatcher.RegisterProvider(blockingProvider{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := dispatcher.Dispatch(ctx, &model.ChatCompletionRequest{Model: "slow-model"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the caller deadline to fail the dispatch")
	}
	if errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatal("a caller-supplied deadline must not be reported as gateway budget exhaustion")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("caller deadline ignored: %v", elapsed)
	}
}

func TestRequestBudgetFromEnv(t *testing.T) {
	cases := map[string]time.Duration{
		"":     DefaultRequestBudget,
		"0":    0,
		"-5":   0,
		"45":   45 * time.Second,
		"oops": 0,
	}
	for raw, want := range cases {
		t.Setenv(RequestBudgetEnv, raw)
		if got := requestBudgetFromEnv(); got != want {
			t.Errorf("requestBudgetFromEnv(%q) = %v, want %v", raw, got, want)
		}
	}
}

// blockingModalityProvider hangs on Embed/Rerank until its context ends, which
// makes the gateway budget observable without any HTTP server.
type blockingModalityProvider struct{}

func (blockingModalityProvider) Name() string             { return "blocking-modality" }
func (blockingModalityProvider) Type() model.ProviderType { return model.ProviderCustom }
func (blockingModalityProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("not used")
}
func (blockingModalityProvider) ChatCompleteStream(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	return nil, errors.New("not used")
}
func (blockingModalityProvider) Embed(ctx context.Context, _ *model.EmbeddingRequest, _ *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingModalityProvider) Rerank(ctx context.Context, _ *model.RerankRequest, _ *model.ChannelConfig) (*model.RerankResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// hangingServer keeps every request open until the test ends. The handler returns
// on the test's release channel, so the budget test stays fast even if the
// cancellation has not reached the server yet (otherwise httptest.Close would
// wait for the handler's own delay).
func hangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv
}

// B-01: GATEWAY_REQUEST_BUDGET_SEC used to bound only non-streaming chat.
// Embedding, rerank and the generic HTTP path then waited for every candidate
// channel to hit its own timeout. Each subtest below uses a 1s budget and a
// channel timeout far above it, so success means the gateway budget (not the
// channel timeout) ended the request.
func TestModalityDispatchStopsAtRequestBudget(t *testing.T) {
	t.Setenv(RequestBudgetEnv, "1")
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "slow", Type: model.ProviderCustom,
		Models:    []string{"emb-model", "rr-model", "img-model"},
		Protocols: []string{"embeddings", "rerank", "images"},
		Status:    "active", Priority: 1, Weight: 1, TimeoutSeconds: 600,
	}})
	d.RegisterProvider(blockingModalityProvider{})

	start := time.Now()
	if _, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "emb-model", Input: "hi"}); !errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatalf("embedding: error = %v, want ErrRequestBudgetExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("embedding dispatch ignored the 1s budget: %v", elapsed)
	}

	start = time.Now()
	if _, err := d.DispatchRerank(context.Background(), &model.RerankRequest{Model: "rr-model", Query: "q", Documents: []any{"d"}}); !errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatalf("rerank: error = %v, want ErrRequestBudgetExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("rerank dispatch ignored the 1s budget: %v", elapsed)
	}

	srv := hangingServer(t)
	httpDispatcher := NewDispatcher([]model.ChannelConfig{{
		Name: "img", Type: model.ProviderOpenAI, BaseURL: srv.URL,
		Models: []string{"img-model"}, Protocols: []string{"images"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 600,
	}})
	start = time.Now()
	if _, err := httpDispatcher.DispatchHTTP(context.Background(), &UpstreamRequest{
		Path: "/v1/images/generations", Method: http.MethodPost,
		Body: []byte(`{"model":"img-model","prompt":"x"}`), ContentType: "application/json",
		Model: "img-model", Protocol: "images",
	}); !errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatalf("http: error = %v, want ErrRequestBudgetExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("http dispatch ignored the 1s budget: %v", elapsed)
	}
}

// B-01 (release timing): a successful DispatchHTTP response body must stay
// readable after the call returns, so the budget cancel may only fire when the
// caller closes the body — exactly like the per-channel timeout.
func TestDispatchHTTPBodySurvivesBudgetRelease(t *testing.T) {
	t.Setenv(RequestBudgetEnv, "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"ok"}]}`))
	}))
	defer srv.Close()

	d := NewDispatcher([]model.ChannelConfig{{
		Name: "img", Type: model.ProviderOpenAI, BaseURL: srv.URL,
		Models: []string{"img-model"}, Protocols: []string{"images"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 600,
	}})
	resp, err := d.DispatchHTTP(context.Background(), &UpstreamRequest{
		Path: "/v1/images/generations", Method: http.MethodPost,
		Body: []byte(`{"model":"img-model","prompt":"x"}`), ContentType: "application/json",
		Model: "img-model", Protocol: "images",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	defer resp.Stream.Close()
	body, err := io.ReadAll(resp.Stream)
	if err != nil {
		t.Fatalf("body read failed after dispatch returned: %v", err)
	}
	if !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("body = %q", body)
	}
}
