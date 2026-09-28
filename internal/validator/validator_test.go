package validator

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func TestOpenAIValidation_Strict(t *testing.T) {
	tempOOB := 2.5
	req := &model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &tempOOB,
	}

	res := ValidateOpenAI(req, nil, LevelStrict)
	if res.Valid {
		t.Fatalf("expected validation failure for out-of-bounds temperature, got valid")
	}
	if res.Error == nil || res.Error.Field != "temperature" {
		t.Fatalf("expected error on field 'temperature', got %v", res.Error)
	}

	// Test reasoning model temperature disallow
	tempOnePointTwo := 1.2
	reasoningReq := &model.ChatCompletionRequest{
		Model: "o1-preview",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Think deep"},
		},
		Temperature: &tempOnePointTwo,
	}
	rule := &config.ModelValidationRule{
		Model:               "o1*",
		DisallowTemperature: true,
	}
	resReasoning := ValidateOpenAI(reasoningReq, rule, LevelStrict)
	if resReasoning.Valid {
		t.Fatalf("expected validation failure for o1 temperature, got valid")
	}
	if resReasoning.Error.Code != "unsupported_parameter" {
		t.Fatalf("expected error code unsupported_parameter, got %s", resReasoning.Error.Code)
	}
}

func TestOpenAIValidation_Lenient(t *testing.T) {
	tempOOB := 3.0
	negMax := -100
	req := &model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "unknown_role", Content: "Test"},
		},
		Temperature: &tempOOB,
		MaxTokens:   &negMax,
	}

	res := ValidateOpenAI(req, nil, LevelLenient)
	if !res.Valid {
		t.Fatalf("expected lenient validation to succeed, got error: %v", res.Error)
	}
	if *req.Temperature != 2.0 {
		t.Fatalf("expected temperature clamped to 2.0, got %f", *req.Temperature)
	}
	if req.MaxTokens != nil {
		t.Fatalf("expected negative max_tokens to be cleared, got %v", *req.MaxTokens)
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("expected unknown_role to be normalized to user, got %s", req.Messages[0].Role)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected lenient warnings to be recorded")
	}
}

func TestAnthropicValidation_Strict(t *testing.T) {
	// 1. Missing max_tokens
	req := &model.AnthropicInboundRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.AnthropicInboundMessage{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: 0,
	}
	res := ValidateAnthropic(req, nil, LevelStrict)
	if res.Valid {
		t.Fatalf("expected failure for missing max_tokens in strict Anthropic mode")
	}
	if res.Error.Field != "max_tokens" {
		t.Fatalf("expected field 'max_tokens', got %s", res.Error.Field)
	}

	// 2. Consecutive turns of same role
	req2 := &model.AnthropicInboundRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []model.AnthropicInboundMessage{
			{Role: "user", Content: "Turn 1"},
			{Role: "user", Content: "Turn 2"},
		},
	}
	res2 := ValidateAnthropic(req2, nil, LevelStrict)
	if res2.Valid {
		t.Fatalf("expected failure for consecutive user turns in strict Anthropic mode")
	}

	// 3. System message in messages array
	req3 := &model.AnthropicInboundRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []model.AnthropicInboundMessage{
			{Role: "system", Content: "You are a bot"},
			{Role: "user", Content: "Hi"},
		},
	}
	res3 := ValidateAnthropic(req3, nil, LevelStrict)
	if res3.Valid {
		t.Fatalf("expected failure for system message in messages array in strict Anthropic mode")
	}
}

func TestAnthropicValidation_Lenient(t *testing.T) {
	tempOOB := 1.5
	req := &model.AnthropicInboundRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.AnthropicInboundMessage{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hello"},
			{Role: "user", Content: "Are you there?"},
		},
		Temperature: &tempOOB,
		MaxTokens:   0,
	}

	res := ValidateAnthropic(req, nil, LevelLenient)
	if !res.Valid {
		t.Fatalf("expected lenient validation to pass, got error: %v", res.Error)
	}
	if req.MaxTokens != 4096 {
		t.Fatalf("expected default max_tokens 4096 injected, got %d", req.MaxTokens)
	}
	if *req.Temperature != 1.0 {
		t.Fatalf("expected temperature clamped to 1.0, got %f", *req.Temperature)
	}
	if req.System != "Be helpful" {
		t.Fatalf("expected system prompt extracted to req.System, got %s", req.System)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected consecutive user turns to be merged into 1, got %d", len(req.Messages))
	}
}

func TestGeminiValidation_Strict(t *testing.T) {
	// 1. Role 'assistant' instead of 'model'
	req := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{Role: "user", Parts: []provider.GeminiPart{{Text: "Hi"}}},
			{Role: "assistant", Parts: []provider.GeminiPart{{Text: "Hello"}}},
		},
	}
	res := ValidateGemini(req, nil, LevelStrict)
	if res.Valid {
		t.Fatalf("expected failure for 'assistant' role in strict Gemini mode")
	}

	// 2. Empty parts
	req2 := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{Role: "user", Parts: []provider.GeminiPart{}},
		},
	}
	res2 := ValidateGemini(req2, nil, LevelStrict)
	if res2.Valid {
		t.Fatalf("expected failure for empty parts in strict Gemini mode")
	}
}

func TestGeminiValidation_Lenient(t *testing.T) {
	tempOOB := 2.5
	req := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{Role: "system", Parts: []provider.GeminiPart{{Text: "System Instruction"}}},
			{Role: "user", Parts: []provider.GeminiPart{{Text: "Part 1"}}},
			{Role: "user", Parts: []provider.GeminiPart{{Text: "Part 2"}}},
			{Role: "assistant", Parts: []provider.GeminiPart{{Text: "Reply"}}},
		},
		GenerationConfig: &provider.GeminiGenerationConfig{
			Temperature: &tempOOB,
		},
	}

	res := ValidateGemini(req, nil, LevelLenient)
	if !res.Valid {
		t.Fatalf("expected lenient validation to pass, got error: %v", res.Error)
	}
	if req.SystemInstruction == nil || len(req.SystemInstruction.Parts) == 0 {
		t.Fatalf("expected system instruction moved out of contents")
	}
	if *req.GenerationConfig.Temperature != 2.0 {
		t.Fatalf("expected temperature clamped to 2.0, got %f", *req.GenerationConfig.Temperature)
	}
	// Contents should be: merged user turn + model turn
	if len(req.Contents) != 2 {
		t.Fatalf("expected 2 contents (user and model), got %d", len(req.Contents))
	}
	if req.Contents[0].Role != "user" || len(req.Contents[0].Parts) != 2 {
		t.Fatalf("expected merged user parts, got %v", req.Contents[0])
	}
	if req.Contents[1].Role != "model" {
		t.Fatalf("expected role assistant converted to model, got %s", req.Contents[1].Role)
	}
}

func TestModelRulePatternMatching(t *testing.T) {
	cfg := &config.Config{
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "lenient",
			Rules: []config.ModelValidationRule{
				{Model: "claude-*", Protocol: "anthropic", Level: "strict"},
				{Model: "o1*", Protocol: "openai", Level: "strict", DisallowTemperature: true},
				{Model: "llama-3-8b-instant", Level: "off"},
			},
		},
	}

	// 1. Claude wildcard
	lvl, rule := cfg.GetValidationRule("claude-3-5-sonnet-20241022", "anthropic")
	if lvl != "strict" || rule == nil {
		t.Fatalf("expected strict for claude-*, got %s", lvl)
	}

	// 2. o1 prefix
	lvlO1, ruleO1 := cfg.GetValidationRule("o1-mini", "openai")
	if lvlO1 != "strict" || ruleO1 == nil || !ruleO1.DisallowTemperature {
		t.Fatalf("expected strict with DisallowTemperature for o1*, got %s, rule: %+v", lvlO1, ruleO1)
	}

	// 3. Exact off
	lvlOff, _ := cfg.GetValidationRule("llama-3-8b-instant", "openai")
	if lvlOff != "off" {
		t.Fatalf("expected off for llama-3-8b-instant, got %s", lvlOff)
	}

	// 4. Default fallback
	lvlDef, _ := cfg.GetValidationRule("unmatched-model", "openai")
	if lvlDef != "lenient" {
		t.Fatalf("expected default lenient, got %s", lvlDef)
	}
}
