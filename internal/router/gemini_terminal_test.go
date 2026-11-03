package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestGeminiBlockedStreamCommitsWithoutTryingAnotherUpstream(t *testing.T) {
	var fallback atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"promptFeedback\":{\"blockReason\":\"SAFETY\"},\"usageMetadata\":{\"promptTokenCount\":7}}\n\n")
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fallback.Add(1); fmt.Fprint(w, "data: [DONE]\n\n") }))
	defer second.Close()
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "google", Type: model.ProviderGemini, BaseURL: first.URL, Models: []string{"m"}, Priority: 1},
		{Name: "other", Type: model.ProviderOpenAI, BaseURL: second.URL, Models: []string{"m"}, Priority: 2},
	})
	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true, Messages: []model.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	filtered := false
	for event := range stream {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Chunk != nil && len(event.Chunk.Choices) > 0 && event.Chunk.Choices[0].FinishReason != nil {
			filtered = *event.Chunk.Choices[0].FinishReason == "content_filter"
		}
	}
	if !filtered || fallback.Load() != 0 {
		t.Fatalf("valid content filter response retried: filtered=%v fallback=%d", filtered, fallback.Load())
	}
}
