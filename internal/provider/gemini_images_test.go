package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestGeminiImageTranslationRejectsUnverifiableInputBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	defer upstream.Close()
	p := NewGeminiProvider(upstream.Client())
	channel := &model.ChannelConfig{Type: model.ProviderGemini, BaseURL: upstream.URL}
	for _, uri := range []string{
		"https://example.com/photo.png", "http://127.0.0.1/private.png",
		"data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,not-base64",
	} {
		for _, stream := range []bool{false, true} {
			req := &model.ChatCompletionRequest{Model: "m", Stream: stream, Messages: []model.ChatMessage{{Role: "user", Content: []model.ContentPart{
				{Type: model.ContentPartImageURL, ImageURL: &model.ImageURLPart{URL: uri}},
			}}}}
			var err error
			if stream {
				_, err = p.ChatCompleteStream(context.Background(), req, channel)
			} else {
				_, err = p.ChatComplete(context.Background(), req, channel)
			}
			var unsupported *UnsupportedTranslationError
			if !errors.As(err, &unsupported) {
				t.Fatalf("image %q stream=%v must fail translation before HTTP: %v", uri, stream, err)
			}
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("unverifiable image reached the upstream %d times", calls.Load())
	}
}

func TestGeminiImageTranslationPreservesInlineMIMEOnWire(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request GeminiRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		image := request.Contents[0].Parts[1].InlineData
		if image == nil || image.MimeType != "image/png" || image.Data != "iVBORw0KGgoAAAANSUhEUg==" {
			t.Errorf("inline PNG lost its MIME or bytes: %#v", image)
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	defer upstream.Close()
	_, err := NewGeminiProvider(upstream.Client()).ChatComplete(context.Background(), typedImageChatRequest(), &model.ChannelConfig{Type: model.ProviderGemini, BaseURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
}
