package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// R-10: Gemini reports some failures as an error object inside a 200 SSE frame;
// it used to be unmarshalled into an empty response and emitted as a normal chunk.
func TestP2GeminiStreamSurfacesErrorFrame(t *testing.T) {
	srv := p2SSEServer(t, "data: {\"error\":{\"code\":503,\"message\":\"The model is overloaded\",\"status\":\"UNAVAILABLE\"}}\n\n")
	p := NewGeminiProvider(srv.Client())
	ch := &model.ChannelConfig{Name: "g", Type: model.ProviderGemini, BaseURL: srv.URL, APIKey: "k", Models: []string{"m"}}

	stream, err := p.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, ch)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	var streamErr error
	for ev := range stream {
		if ev.Err != nil {
			streamErr = ev.Err
		}
	}
	if streamErr == nil {
		t.Fatalf("the upstream error frame was swallowed: a truncated stream looked successful")
	}
}

// R-10 (unary half): a 200 body carrying an error object used to become an empty
// successful completion.
func TestP2GeminiUnarySurfacesErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":{"code":503,"message":"The model is overloaded","status":"UNAVAILABLE"}}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(srv.Client())
	ch := &model.ChannelConfig{Name: "g", Type: model.ProviderGemini, BaseURL: srv.URL, APIKey: "k", Models: []string{"m"}}
	resp, err := p.ChatComplete(context.Background(), &model.ChatCompletionRequest{Model: "m"}, ch)
	if err == nil {
		t.Fatalf("upstream error body returned a successful empty completion: %+v", resp)
	}
}
