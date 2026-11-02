package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func validateGeminiChatOptions(req *model.ChatCompletionRequest) error {
	for name := range req.Extra {
		switch name {
		case "response_format", "max_completion_tokens", "seed", "reasoning_effort":
		default:
			return &UnsupportedTranslationError{Reason: fmt.Sprintf("Chat option %q has no supported Gemini translation", name)}
		}
	}
	for _, tool := range req.Tools {
		if tool.Type != "function" {
			return &UnsupportedTranslationError{Reason: "this tool requires a native upstream"}
		}
	}
	return nil
}

func geminiGenerationConfig(req *model.ChatCompletionRequest) *GeminiGenerationConfig {
	cfg := &GeminiGenerationConfig{
		Temperature: req.Temperature, TopP: req.TopP, MaxOutputTokens: req.MaxTokens,
		CandidateCount: req.N, PresencePenalty: req.PresencePenalty, FrequencyPenalty: req.FrequencyPenalty,
	}
	if raw := req.Extra["max_completion_tokens"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg.MaxOutputTokens)
	}
	if raw := req.Extra["seed"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg.Seed)
	}
	switch stop := req.Stop.(type) {
	case string:
		cfg.StopSequences = []string{stop}
	case []string:
		cfg.StopSequences = stop
	case []any:
		for _, value := range stop {
			if text, ok := value.(string); ok {
				cfg.StopSequences = append(cfg.StopSequences, text)
			}
		}
	}
	var format struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	}
	if json.Unmarshal(req.Extra["response_format"], &format) == nil {
		switch format.Type {
		case "json_object":
			cfg.ResponseMIMEType = "application/json"
		case "json_schema":
			cfg.ResponseMIMEType, cfg.ResponseJSONSchema = "application/json", format.JSONSchema.Schema
		}
	}
	return cfg
}

func applyGeminiThinking(cfg *GeminiGenerationConfig, req *model.ChatCompletionRequest, upstreamModel string) error {
	raw := req.Extra["reasoning_effort"]
	if len(raw) == 0 {
		return nil
	}
	var effort string
	if json.Unmarshal(raw, &effort) != nil {
		return &UnsupportedTranslationError{Reason: "reasoning_effort must be a string"}
	}
	if effort != "none" && effort != "minimal" && effort != "low" && effort != "medium" && effort != "high" {
		return &UnsupportedTranslationError{Reason: fmt.Sprintf("unsupported Gemini reasoning_effort %q", effort)}
	}
	name := strings.ToLower(upstreamModel)
	cfg.ThinkingConfig = &GeminiThinkingConfig{}
	switch {
	case strings.Contains(name, "gemini-2.5"):
		budget := map[string]int{"none": 0, "minimal": 1024, "low": 1024, "medium": 8192, "high": 24576}[effort]
		cfg.ThinkingConfig.ThinkingBudget = &budget
	case strings.Contains(name, "gemma"):
		cfg.ThinkingConfig.ThinkingLevel = "high"
		if effort == "none" || effort == "minimal" {
			cfg.ThinkingConfig.ThinkingLevel = "minimal"
		}
	default:
		if effort == "none" {
			return &UnsupportedTranslationError{Reason: "this Gemini model does not support disabling thinking"}
		}
		if strings.Contains(name, "gemini-4") && effort == "minimal" {
			effort = "low"
		}
		cfg.ThinkingConfig.ThinkingLevel = effort
	}
	return nil
}
