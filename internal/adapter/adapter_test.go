package adapter

import (
	"context"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func TestAnthropicAdapter_Bidirectional(t *testing.T) {
	ad := NewAnthropicAdapter()
	if ad.Protocol() != "anthropic" {
		t.Fatalf("expected protocol 'anthropic', got '%s'", ad.Protocol())
	}

	maxTokens := 2048
	inbound := &model.AnthropicInboundRequest{
		Model:     "claude-3-5-sonnet",
		System:    "You are a helpful coding assistant.",
		MaxTokens: maxTokens,
		Messages: []model.AnthropicInboundMessage{
			{Role: "user", Content: "Hello world"},
		},
	}

	// 1. ToCanonical
	canonical, err := ad.ToCanonical(context.Background(), inbound)
	if err != nil {
		t.Fatalf("ToCanonical failed: %v", err)
	}
	if len(canonical.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(canonical.Messages))
	}
	if canonical.Messages[0].Role != "system" || canonical.Messages[1].Role != "user" {
		t.Fatalf("unexpected message roles: %+v", canonical.Messages)
	}

	// 2. FromCanonical
	stop := "stop"
	canonResp := &model.ChatCompletionResponse{
		ID:    "chatcmpl-test",
		Model: "claude-3-5-sonnet",
		Choices: []model.ChatCompletionChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:    "assistant",
					Content: "Hello! How can I assist you today?",
				},
				FinishReason: &stop,
			},
		},
		Usage: &model.Usage{
			PromptTokens:     10,
			CompletionTokens: 15,
		},
	}

	anthropicOut, err := ad.FromCanonical(context.Background(), canonResp)
	if err != nil {
		t.Fatalf("FromCanonical failed: %v", err)
	}
	outMap, ok := anthropicOut.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any output, got %T", anthropicOut)
	}
	if outMap["role"] != "assistant" || outMap["stop_reason"] != "end_turn" {
		t.Fatalf("unexpected Anthropic output fields: %+v", outMap)
	}
}

func TestGeminiAdapter_Bidirectional(t *testing.T) {
	ad := NewGeminiAdapter()
	if ad.Protocol() != "gemini" {
		t.Fatalf("expected protocol 'gemini', got '%s'", ad.Protocol())
	}

	temp := 0.7
	inbound := &provider.GeminiRequest{
		SystemInstruction: &provider.GeminiSystemInstruction{
			Parts: []provider.GeminiPart{{Text: "System prompt"}},
		},
		Contents: []provider.GeminiContent{
			{
				Role:  "user",
				Parts: []provider.GeminiPart{{Text: "Explain quantum computing"}},
			},
		},
		GenerationConfig: &provider.GeminiGenerationConfig{
			Temperature: &temp,
		},
	}

	// 1. ToCanonical
	canonical, err := ad.ToCanonical(context.Background(), inbound)
	if err != nil {
		t.Fatalf("ToCanonical failed: %v", err)
	}
	if len(canonical.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(canonical.Messages))
	}
	if canonical.Messages[0].Role != "system" || canonical.Messages[1].Role != "user" {
		t.Fatalf("unexpected message roles: %+v", canonical.Messages)
	}

	// 2. FromCanonical
	canonResp := &model.ChatCompletionResponse{
		ID:    "chatcmpl-gemini",
		Model: "gemini-1.5-pro",
		Choices: []model.ChatCompletionChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:    "assistant",
					Content: "Quantum computing is...",
				},
			},
		},
	}

	geminiOut, err := ad.FromCanonical(context.Background(), canonResp)
	if err != nil {
		t.Fatalf("FromCanonical failed: %v", err)
	}
	outMap, ok := geminiOut.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any output, got %T", geminiOut)
	}
	candidates, ok := outMap["candidates"].([]provider.GeminiCandidate)
	if !ok || len(candidates) != 1 {
		t.Fatalf("expected 1 candidate in output, got %v", outMap["candidates"])
	}
	if candidates[0].Content.Role != "model" {
		t.Fatalf("expected candidate role 'model', got '%s'", candidates[0].Content.Role)
	}
}
