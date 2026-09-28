package validator

import (
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// Pipeline orchestrates protocol validation steps according to configured per-model rules.
type Pipeline struct {
	cfg *config.Config
}

// NewPipeline creates a new validation pipeline.
func NewPipeline(cfg *config.Config) *Pipeline {
	if cfg == nil {
		cfg = config.GetGlobalConfig()
	}
	return &Pipeline{cfg: cfg}
}

// ValidateOpenAI validates an OpenAI request against configured model and key rules.
func (p *Pipeline) ValidateOpenAI(req *model.ChatCompletionRequest, keyCfg ...*model.APIKeyConfig) *ValidationResult {
	if req == nil {
		return &ValidationResult{Valid: true, Level: LevelOff}
	}
	cfg := p.cfg
	if cfg == nil {
		cfg = config.GetGlobalConfig()
	}
	var k *model.APIKeyConfig
	if len(keyCfg) > 0 {
		k = keyCfg[0]
	}
	level, rule := cfg.ResolveValidationLevel(req.Model, ProtocolOpenAI, k)
	return ValidateOpenAI(req, rule, level)
}

// ValidateAnthropic validates an Anthropic request against configured model and key rules.
func (p *Pipeline) ValidateAnthropic(req *model.AnthropicInboundRequest, keyCfg ...*model.APIKeyConfig) *ValidationResult {
	if req == nil {
		return &ValidationResult{Valid: true, Level: LevelOff}
	}
	cfg := p.cfg
	if cfg == nil {
		cfg = config.GetGlobalConfig()
	}
	var k *model.APIKeyConfig
	if len(keyCfg) > 0 {
		k = keyCfg[0]
	}
	level, rule := cfg.ResolveValidationLevel(req.Model, ProtocolAnthropic, k)
	return ValidateAnthropic(req, rule, level)
}

// ValidateGemini validates a Gemini request against configured model and key rules.
func (p *Pipeline) ValidateGemini(modelName string, req *provider.GeminiRequest, keyCfg ...*model.APIKeyConfig) *ValidationResult {
	if req == nil {
		return &ValidationResult{Valid: true, Level: LevelOff}
	}
	cfg := p.cfg
	if cfg == nil {
		cfg = config.GetGlobalConfig()
	}
	var k *model.APIKeyConfig
	if len(keyCfg) > 0 {
		k = keyCfg[0]
	}
	level, rule := cfg.ResolveValidationLevel(modelName, ProtocolGemini, k)
	return ValidateGemini(req, rule, level)
}

// ValidateOpenAIRequest validates an OpenAI request using global configuration and optional key config.
func ValidateOpenAIRequest(req *model.ChatCompletionRequest, keyCfg ...*model.APIKeyConfig) *ValidationResult {
	cfg := config.GetGlobalConfig()
	var k *model.APIKeyConfig
	if len(keyCfg) > 0 {
		k = keyCfg[0]
	}
	level, rule := cfg.ResolveValidationLevel(req.Model, ProtocolOpenAI, k)
	return ValidateOpenAI(req, rule, level)
}

// ValidateAnthropicRequest validates an Anthropic request using global configuration and optional key config.
func ValidateAnthropicRequest(req *model.AnthropicInboundRequest, keyCfg ...*model.APIKeyConfig) *ValidationResult {
	cfg := config.GetGlobalConfig()
	var k *model.APIKeyConfig
	if len(keyCfg) > 0 {
		k = keyCfg[0]
	}
	level, rule := cfg.ResolveValidationLevel(req.Model, ProtocolAnthropic, k)
	return ValidateAnthropic(req, rule, level)
}

// ValidateGeminiRequest validates a Gemini request using global configuration and optional key config.
func ValidateGeminiRequest(modelName string, req *provider.GeminiRequest, keyCfg ...*model.APIKeyConfig) *ValidationResult {
	cfg := config.GetGlobalConfig()
	var k *model.APIKeyConfig
	if len(keyCfg) > 0 {
		k = keyCfg[0]
	}
	level, rule := cfg.ResolveValidationLevel(modelName, ProtocolGemini, k)
	return ValidateGemini(req, rule, level)
}
