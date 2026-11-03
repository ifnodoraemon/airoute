package adapter

import (
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func applyGeminiConfig(request *provider.GeminiRequest, chat *model.ChatCompletionRequest) {
	chat.ToolChoice = geminiCanonicalToolChoice(request.ToolConfig)
	cfg := request.GenerationConfig
	if cfg == nil {
		return
	}
	chat.N, chat.Stop = cfg.CandidateCount, cfg.StopSequences
	chat.PresencePenalty, chat.FrequencyPenalty = cfg.PresencePenalty, cfg.FrequencyPenalty
	chat.Extra = make(model.ExtraFields)
	if cfg.Seed != nil {
		chat.Extra["seed"], _ = json.Marshal(*cfg.Seed)
	}
	if cfg.ResponseMIMEType == "application/json" {
		format := map[string]any{"type": "json_object"}
		schema := cfg.ResponseJSONSchema
		if len(schema) == 0 {
			schema = cfg.ResponseSchema
		}
		if len(schema) > 0 {
			format = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "response", "schema": schema}}
		}
		chat.Extra["response_format"], _ = json.Marshal(format)
	}
	if cfg.ThinkingConfig != nil && cfg.ThinkingConfig.ThinkingLevel != "" {
		chat.Extra["reasoning_effort"], _ = json.Marshal(cfg.ThinkingConfig.ThinkingLevel)
	}
}

func geminiCanonicalToolChoice(config *provider.GeminiToolConfig) any {
	if config == nil || config.FunctionCallingConfig == nil {
		return nil
	}
	choice := config.FunctionCallingConfig
	switch choice.Mode {
	case "NONE":
		return "none"
	case "ANY":
		if len(choice.AllowedFunctionNames) == 1 {
			return map[string]any{"type": "function", "function": map[string]any{"name": choice.AllowedFunctionNames[0]}}
		}
		return "required"
	default:
		return "auto"
	}
}

func geminiReasoningTokens(response *model.ChatCompletionResponse) int {
	if response.Usage == nil {
		return 0
	}
	return response.Usage.GetReasoningTokens()
}

func geminiCachedTokens(response *model.ChatCompletionResponse) int {
	if response.Usage == nil {
		return 0
	}
	return response.Usage.GetCachedTokens()
}

func takeGeminiToolID(pending map[string][]string, name, explicit string) string {
	queue := pending[name]
	id := explicit
	if id == "" && len(queue) > 0 {
		id = queue[0]
	}
	if id == "" && name != "" {
		id = "call_" + name
	}
	for i, queued := range queue {
		if queued == id {
			pending[name] = append(queue[:i], queue[i+1:]...)
			break
		}
	}
	return id
}
