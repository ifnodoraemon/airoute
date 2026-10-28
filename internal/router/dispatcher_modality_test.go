package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

const (
	embeddingOKBody = `{"object":"list","data":[{"object":"embedding","embedding":[0.1],"index":0}],"model":"m","usage":{"prompt_tokens":3,"total_tokens":3}}`
	rerankOKBody    = `{"results":[{"index":0,"relevance_score":0.5}],"usage":{"total_tokens":5}}`
)

// jsonStatusServer answers every request with one status/body, counting hits so a
// test can assert that a channel was (or was not) called.
func jsonStatusServer(t *testing.T, status int, body string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// B-03: a 404 means "this node does not host the model", not "the request is
// wrong". Embedding and rerank used to classify every 4xx rejection as a client
// error and abort the whole fan-out on the first node that did not host the model.
func TestModalityDispatchContinuesAfterNodeSpecific404(t *testing.T) {
	t.Run("embedding", func(t *testing.T) {
		notFound, _ := jsonStatusServer(t, http.StatusNotFound, `{"error":"model not hosted on this node"}`)
		ok, okHits := jsonStatusServer(t, http.StatusOK, embeddingOKBody)
		d := NewDispatcher([]model.ChannelConfig{
			{Name: "node-a", Type: model.ProviderOpenAI, BaseURL: notFound.URL, Models: []string{"emb-model"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 1, Weight: 1},
			{Name: "node-b", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"emb-model"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 2, Weight: 1},
		})

		resp, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "emb-model", Input: "hi"})
		if err != nil {
			t.Fatalf("embedding dispatch aborted on a node-specific 404: %v", err)
		}
		if resp.Channel != "node-a→node-b" {
			t.Fatalf("channel trail = %q, want node-a→node-b", resp.Channel)
		}
		if atomic.LoadInt32(okHits) != 1 {
			t.Fatalf("second node hits = %d, want 1", *okHits)
		}
	})

	t.Run("rerank", func(t *testing.T) {
		notFound, _ := jsonStatusServer(t, http.StatusNotFound, `{"error":"model not hosted on this node"}`)
		ok, _ := jsonStatusServer(t, http.StatusOK, rerankOKBody)
		d := NewDispatcher([]model.ChannelConfig{
			{Name: "node-a", Type: model.ProviderOpenAI, BaseURL: notFound.URL, Models: []string{"rr-model"}, Protocols: []string{"rerank"}, Status: "active", Priority: 1, Weight: 1},
			{Name: "node-b", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"rr-model"}, Protocols: []string{"rerank"}, Status: "active", Priority: 2, Weight: 1},
		})

		resp, err := d.DispatchRerank(context.Background(), &model.RerankRequest{Model: "rr-model", Query: "q", Documents: []any{"d"}})
		if err != nil {
			t.Fatalf("rerank dispatch aborted on a node-specific 404: %v", err)
		}
		if resp.Channel != "node-a→node-b" {
			t.Fatalf("channel trail = %q, want node-a→node-b", resp.Channel)
		}
	})
}

// B-03 (negative case): a deterministic payload error (400/413/422) must still
// end the fan-out, because replaying it on another channel cannot help.
func TestModalityDispatchStopsOnFatalPayloadError(t *testing.T) {
	bad, _ := jsonStatusServer(t, http.StatusBadRequest, `{"error":"bad input"}`)
	ok, okHits := jsonStatusServer(t, http.StatusOK, embeddingOKBody)
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "node-a", Type: model.ProviderOpenAI, BaseURL: bad.URL, Models: []string{"emb-model"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 1, Weight: 1},
		{Name: "node-b", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"emb-model"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 2, Weight: 1},
	})

	_, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "emb-model", Input: "hi"})
	if err == nil {
		t.Fatal("expected the deterministic payload error to surface")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error = %v, want the upstream 400 surfaced", err)
	}
	if hits := atomic.LoadInt32(okHits); hits != 0 {
		t.Fatalf("a fatal payload error was replayed on %d other channel(s)", hits)
	}
}

// B-11: the cross-model fallback table used to be consulted only by the chat
// paths. Embeddings, rerank and generic HTTP now honour it too, with the same
// cycle/depth guard.
func TestModalityDispatchUsesCrossModelFallback(t *testing.T) {
	t.Run("no route", func(t *testing.T) {
		ok, _ := jsonStatusServer(t, http.StatusOK, embeddingOKBody)
		d := NewDispatcher([]model.ChannelConfig{
			{Name: "backup", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"backup-emb"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 1, Weight: 1},
		})
		d.SetModelFallbacks(map[string]string{"primary-emb": "backup-emb"})

		resp, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "primary-emb", Input: "hi"})
		if err != nil {
			t.Fatalf("embedding fallback was not used: %v", err)
		}
		if resp.Channel != "backup" {
			t.Fatalf("channel = %q, want the fallback model's channel", resp.Channel)
		}
	})

	t.Run("exhausted", func(t *testing.T) {
		failing, _ := jsonStatusServer(t, http.StatusInternalServerError, `{"error":"boom"}`)
		ok, _ := jsonStatusServer(t, http.StatusOK, embeddingOKBody)
		d := NewDispatcher([]model.ChannelConfig{
			{Name: "primary", Type: model.ProviderOpenAI, BaseURL: failing.URL, Models: []string{"primary-emb"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 1, Weight: 1},
			{Name: "backup", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"backup-emb"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 1, Weight: 1},
		})
		d.SetModelFallbacks(map[string]string{"primary-emb": "backup-emb"})

		resp, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "primary-emb", Input: "hi"})
		if err != nil {
			t.Fatalf("embedding fallback was not used after every candidate failed: %v", err)
		}
		if resp.Channel != "backup" {
			t.Fatalf("channel = %q, want the fallback model's channel", resp.Channel)
		}
	})

	t.Run("cycle guard", func(t *testing.T) {
		d := NewDispatcher(nil)
		d.SetModelFallbacks(map[string]string{"a-emb": "b-emb", "b-emb": "a-emb"})

		_, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "a-emb", Input: "hi"})
		if err == nil {
			t.Fatal("expected no-route error for a cyclic fallback chain")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "no upstream provider") {
			t.Fatalf("error = %v, want the no-route failure", err)
		}
	})

	t.Run("chain", func(t *testing.T) {
		ok, _ := jsonStatusServer(t, http.StatusOK, embeddingOKBody)
		d := NewDispatcher([]model.ChannelConfig{
			{Name: "third", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"c-emb"}, Protocols: []string{"embeddings"}, Status: "active", Priority: 1, Weight: 1},
		})
		d.SetModelFallbacks(map[string]string{"a-emb": "b-emb", "b-emb": "c-emb"})

		resp, err := d.DispatchEmbedding(context.Background(), &model.EmbeddingRequest{Model: "a-emb", Input: "hi"})
		if err != nil {
			t.Fatalf("chained fallback a-emb -> b-emb -> c-emb was not followed: %v", err)
		}
		if resp.Channel != "third" {
			t.Fatalf("channel = %q, want the final fallback model's channel", resp.Channel)
		}
	})
}

// B-11 (HTTP path): the generic modality path honours the fallback chain as well,
// and a pinned channel bypasses it (operator override).
func TestDispatchHTTPUsesCrossModelFallback(t *testing.T) {
	ok, _ := jsonStatusServer(t, http.StatusOK, `{"data":[{"url":"ok"}]}`)
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "backup", Type: model.ProviderOpenAI, BaseURL: ok.URL, Models: []string{"backup-img"}, Protocols: []string{"images"}, Status: "active", Priority: 1, Weight: 1},
	})
	d.SetModelFallbacks(map[string]string{"primary-img": "backup-img"})

	resp, err := d.DispatchHTTP(context.Background(), &UpstreamRequest{
		Path: "/v1/images/generations", Method: http.MethodPost,
		Body: []byte(`{"model":"primary-img","prompt":"x"}`), ContentType: "application/json",
		Model: "primary-img", Protocol: "images",
	})
	if err != nil {
		t.Fatalf("http fallback was not used: %v", err)
	}
	if resp.Channel != "backup" {
		t.Fatalf("channel = %q, want the fallback model's channel", resp.Channel)
	}
	if resp.Stream != nil {
		_ = resp.Stream.Close()
	}
}
