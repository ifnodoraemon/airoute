package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestGPUStackAndCustomProviderRouting(t *testing.T) {
	// Setup a mock GPUStack server
	mockGPUStackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-gpu-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		stop := "stop"
		resp := model.ChatCompletionResponse{
			ID:      "chatcmpl-gpustack-12345",
			Object:  "chat.completion",
			Created: 1700000000,
			Model:   "meta-llama/Llama-3.1-8B-Instruct",
			Choices: []model.ChatCompletionChoice{
				{
					Index: 0,
					Message: model.ChatMessage{
						Role:    "assistant",
						Content: "Hello from GPUStack localized compute cluster!",
					},
					FinishReason: &stop,
				},
			},
			Usage: &model.Usage{
				PromptTokens:     10,
				CompletionTokens: 8,
				TotalTokens:      18,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockGPUStackServer.Close()

	// 1. Configure GPUStack and Sub2API channels
	channels := []model.ChannelConfig{
		{
			Name:      "gpustack-primary",
			Type:      model.ProviderGPUStack,
			BaseURL:   mockGPUStackServer.URL,
			APIKey:    "test-gpu-key",
			Models:    []string{"meta-llama/Llama-3.1-8B-Instruct"},
			Protocols: []string{"openai_chat", "openai_text"},
			Priority:  1,
			Weight:    10,
		},
		{
			Name:      "sub2api-backup",
			Type:      model.ProviderSub2API,
			BaseURL:   "http://127.0.0.1:9999",
			APIKey:    "test-sub2api-key",
			Models:    []string{"meta-llama/Llama-3.1-8B-Instruct"},
			Protocols: []string{"anthropic_messages"},
			Priority:  2,
			Weight:    10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	// 2. Test protocol filtering
	chatChannels := dispatcher.GetChannelsForModelAndProtocol("meta-llama/Llama-3.1-8B-Instruct", "openai_chat")
	if len(chatChannels) != 1 {
		t.Fatalf("expected 1 channel supporting openai_chat, got %d", len(chatChannels))
	}
	if chatChannels[0].Name != "gpustack-primary" {
		t.Fatalf("expected gpustack-primary, got %s", chatChannels[0].Name)
	}

	messagesChannels := dispatcher.GetChannelsForModelAndProtocol("meta-llama/Llama-3.1-8B-Instruct", "anthropic_messages")
	if len(messagesChannels) != 1 {
		t.Fatalf("expected 1 channel supporting anthropic_messages, got %d", len(messagesChannels))
	}
	if messagesChannels[0].Name != "sub2api-backup" {
		t.Fatalf("expected sub2api-backup, got %s", messagesChannels[0].Name)
	}

	// 3. Test end-to-end Dispatch to GPUStack
	req := &model.ChatCompletionRequest{
		Model: "meta-llama/Llama-3.1-8B-Instruct",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello cluster"},
		},
	}

	resp, err := dispatcher.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error calling GPUStack: %v", err)
	}

	content := resp.Choices[0].Message.GetContentString()
	expected := "Hello from GPUStack localized compute cluster!"
	if content != expected {
		t.Fatalf("expected '%s', got '%s'", expected, content)
	}

	// 4. Test Circuit Breaker reporting
	status := dispatcher.GetBreakerStatus("gpustack-primary")
	if status != "CLOSED" {
		t.Fatalf("expected CLOSED status for healthy GPUStack, got %s", status)
	}
}
