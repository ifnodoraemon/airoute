package validator

import (
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

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
