package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// TestAutoProbe_OpenAIAndGPUStack verifies that the DownstreamProber correctly discovers models and protocols.
func TestAutoProbe_OpenAIAndGPUStack(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Server", "GPUStack/v1.2.0")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "meta-llama/Llama-3.1-8B-Instruct"},
					{"id": "dall-e-3"},
					{"id": "tts-1"},
					{"id": "whisper-1"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	prober := controlplane.NewDownstreamProber(mockServer.Client())
	res, err := prober.Probe(context.Background(), &controlplane.ProbeRequest{
		BaseURL: mockServer.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	if res.Type != model.ProviderGPUStack {
		t.Errorf("expected detected type %s, got %s", model.ProviderGPUStack, res.Type)
	}
	if len(res.Models) != 4 {
		t.Fatalf("expected 4 models, got %d", len(res.Models))
	}

	// Verify inferred protocols include images, tts, stt
	hasImages := false
	hasTTS := false
	hasSTT := false
	for _, p := range res.Protocols {
		if p == "images" {
			hasImages = true
		}
		if p == "audio_speech" {
			hasTTS = true
		}
		if p == "audio_transcription" {
			hasSTT = true
		}
	}
	if !hasImages || !hasTTS || !hasSTT {
		t.Errorf("expected auto-detected protocols to include images, audio_speech, audio_transcription; got %v", res.Protocols)
	}
}

