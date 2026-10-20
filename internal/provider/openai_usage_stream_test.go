package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestGPUStackStreamRequestsAndRetainsUsageTrailer(t *testing.T) {
	for _, protocol := range []string{"openai_chat", "openai_text"} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					StreamOptions *model.StreamOptions `json:"stream_options"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.StreamOptions == nil || !request.StreamOptions.IncludeUsage {
					t.Error("upstream usage not requested")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				// Some gateways close directly after the usage frame, with no newline.
				fmt.Fprint(w, `data: {"id":"gpu-1","choices":[],"usage":{"prompt_tokens":32,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":16}}}`)
			}))
			defer server.Close()
			channel := &model.ChannelConfig{Name: "gpustack", Type: model.ProviderGPUStack, BaseURL: server.URL, Protocols: []string{protocol}}
			events, err := NewOpenAIProvider(server.Client()).ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, channel)
			if err != nil {
				t.Fatal(err)
			}
			var usage *model.Usage
			for event := range events {
				if event.Err != nil {
					t.Fatal(event.Err)
				}
				if event.Chunk != nil && event.Chunk.Usage != nil {
					usage = event.Chunk.Usage
				}
			}
			if usage == nil || usage.PromptTokens != 32 || usage.GetCachedTokens() != 16 {
				t.Fatalf("lost trailer usage: %+v", usage)
			}
		})
	}
}
