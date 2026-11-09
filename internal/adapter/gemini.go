package adapter

import (
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// GeminiToCanonical converts a typed Gemini request into a chat request.
func GeminiToCanonical(req *provider.GeminiRequest) (*model.ChatCompletionRequest, error) {

	var msgs []model.ChatMessage
	if req.SystemInstruction != nil {
		var sysText string
		for _, p := range req.SystemInstruction.Parts {
			sysText += p.Text
		}
		if sysText != "" {
			msgs = append(msgs, model.ChatMessage{
				Role:    "system",
				Content: sysText,
			})
		}
	}

	conversation := geminiConversation{pendingToolIDs: make(map[string][]string)}
	for _, content := range req.Contents {
		msgs = append(msgs, conversation.messages(content)...)
	}

	var temp *float64
	var topP *float64
	var maxTokens *int
	if req.GenerationConfig != nil {
		temp = req.GenerationConfig.Temperature
		topP = req.GenerationConfig.TopP
		maxTokens = req.GenerationConfig.MaxOutputTokens
	}

	tools := geminiCanonicalTools(req)

	canonical := &model.ChatCompletionRequest{
		Messages:    msgs,
		Temperature: temp,
		TopP:        topP,
		MaxTokens:   maxTokens,
		Tools:       tools,
	}
	applyGeminiConfig(req, canonical)
	return canonical, nil
}

func geminiCanonicalTools(req *provider.GeminiRequest) []model.Tool {
	var tools []model.Tool
	for _, tc := range req.Tools {
		for _, fd := range tc.FunctionDeclarations {
			params := fd.Parameters
			if fd.ParametersJSONSchema != nil {
				params = fd.ParametersJSONSchema
			}
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, model.Tool{
				Type: "function",
				Function: map[string]any{
					"name":        fd.Name,
					"description": fd.Description,
					"parameters":  params,
				},
			})
		}
	}

	return tools
}
