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

func TestAnthropicAdapter_ToolResultWithText(t *testing.T) {
	ad := NewAnthropicAdapter()
	inbound := &model.AnthropicInboundRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.AnthropicInboundMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "tool_123",
						"content":     "Weather is 22C and sunny",
					},
					map[string]any{
						"type": "text",
						"text": "What should I wear?",
					},
				},
			},
		},
	}

	canonical, err := ad.ToCanonical(context.Background(), inbound)
	if err != nil {
		t.Fatalf("ToCanonical failed: %v", err)
	}
	if len(canonical.Messages) != 2 {
		t.Fatalf("expected 2 messages (tool + user), got %d", len(canonical.Messages))
	}
	if canonical.Messages[0].Role != "tool" || canonical.Messages[0].ToolCallID != "tool_123" {
		t.Errorf("unexpected first message: %+v", canonical.Messages[0])
	}
	if canonical.Messages[1].Role != "user" || canonical.Messages[1].Content != "What should I wear?" {
		t.Errorf("unexpected second message: %+v", canonical.Messages[1])
	}
}

func TestGeminiAdapter_ToolCalls(t *testing.T) {
	ad := NewGeminiAdapter()
	inbound := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{
				Role: "model",
				Parts: []provider.GeminiPart{
					{
						FunctionCall: &provider.GeminiFunctionCall{
							Name: "get_weather",
							Args: map[string]any{"city": "Paris"},
						},
					},
				},
			},
		},
	}

	canonical, err := ad.ToCanonical(context.Background(), inbound)
	if err != nil {
		t.Fatalf("ToCanonical failed: %v", err)
	}
	if len(canonical.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(canonical.Messages))
	}
	msg := canonical.Messages[0]
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("unexpected function name: %s", msg.ToolCalls[0].Function.Name)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("unexpected function arguments: %s", msg.ToolCalls[0].Function.Arguments)
	}

	// FromCanonical with tool calls
	idx := 0
	canonResp := &model.ChatCompletionResponse{
		ID:    "test-gemini-tools",
		Model: "gemini-1.5-flash",
		Choices: []model.ChatCompletionChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role: "assistant",
					ToolCalls: []model.ToolCall{
						{
							Index: &idx,
							ID:    "call_1",
							Type:  "function",
							Function: model.FunctionCall{
								Name:      "search",
								Arguments: `{"query":"golang"}`,
							},
						},
					},
				},
			},
		},
	}

	geminiOut, err := ad.FromCanonical(context.Background(), canonResp)
	if err != nil {
		t.Fatalf("FromCanonical failed: %v", err)
	}
	outMap := geminiOut.(map[string]any)
	candidates := outMap["candidates"].([]provider.GeminiCandidate)
	if len(candidates) != 1 || len(candidates[0].Content.Parts) != 1 {
		t.Fatalf("unexpected candidate parts: %+v", candidates)
	}
	fnCall := candidates[0].Content.Parts[0].FunctionCall
	if fnCall == nil || fnCall.Name != "search" || fnCall.Args["query"] != "golang" {
		t.Errorf("unexpected FunctionCall in Gemini response: %+v", fnCall)
	}
}
