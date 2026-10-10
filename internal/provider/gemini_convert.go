package provider

import (
	"encoding/json"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// convertOpenAIToGemini translates OpenAI canonical request to Gemini structure.
func convertOpenAIToGemini(req *model.ChatCompletionRequest) *GeminiRequest {
	var contents []GeminiContent
	var systemParts []GeminiPart

	toolCallNames := make(map[string]string)
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			if tc.ID != "" && tc.Function.Name != "" {
				toolCallNames[tc.ID] = tc.Function.Name
			}
		}
	}

	for _, msg := range req.Messages {
		if strings.ToLower(msg.Role) == "system" {
			parts := model.ParseMessageContent(msg.Content)
			for _, part := range parts {
				if part.Text != "" {
					systemParts = append(systemParts, GeminiPart{Text: part.Text})
				}
			}
			continue
		}

		if strings.ToLower(msg.Role) == "tool" {
			var respMap map[string]any
			if err := json.Unmarshal([]byte(msg.GetContentString()), &respMap); err != nil {
				respMap = map[string]any{"content": msg.GetContentString()}
			}
			fnName := msg.Name
			if fnName == "" && msg.ToolCallID != "" {
				fnName = toolCallNames[msg.ToolCallID]
			}
			if fnName == "" {
				fnName = "function_response"
			}
			contents = append(contents, GeminiContent{
				Role: "user",
				Parts: []GeminiPart{
					{
						FunctionResponse: &GeminiFunctionResponse{
							Name:     fnName,
							Response: respMap,
						},
					},
				},
			})
			continue
		}

		if strings.ToLower(msg.Role) == "assistant" && len(msg.ToolCalls) > 0 {
			var geminiParts []GeminiPart
			if text := msg.GetContentString(); text != "" {
				geminiParts = append(geminiParts, GeminiPart{Text: text})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				geminiParts = append(geminiParts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}
			contents = append(contents, GeminiContent{
				Role:  "model",
				Parts: geminiParts,
			})
			continue
		}

		parts := model.ParseMessageContent(msg.Content)
		if len(parts) == 0 {
			continue
		}

		// Role mapping: "assistant" -> "model", "user" -> "user"
		role := "user"
		if strings.ToLower(msg.Role) == "assistant" {
			role = "model"
		}

		var geminiParts []GeminiPart
		for _, part := range parts {
			switch part.Type {
			case model.ContentPartText:
				if part.Text != "" {
					geminiParts = append(geminiParts, GeminiPart{Text: part.Text})
				}
			case model.ContentPartImageURL:
				if part.ImageURL != nil && part.ImageURL.URL != "" {
					mime, b64 := model.ParseDataURI(part.ImageURL.URL)
					geminiParts = append(geminiParts, GeminiPart{
						InlineData: &GeminiInlineData{
							MimeType: mime,
							Data:     b64,
						},
					})
				}
			case model.ContentPartInputAudio:
				if part.InputAudio != nil && part.InputAudio.Data != "" {
					mime := "audio/wav"
					if part.InputAudio.Format == "mp3" {
						mime = "audio/mp3"
					}
					geminiParts = append(geminiParts, GeminiPart{
						InlineData: &GeminiInlineData{
							MimeType: mime,
							Data:     part.InputAudio.Data,
						},
					})
				}
			}
		}

		if len(geminiParts) > 0 {
			contents = append(contents, GeminiContent{
				Role:  role,
				Parts: geminiParts,
			})
		}
	}

	var toolContainers []GeminiToolDeclarationContainer
	if len(req.Tools) > 0 {
		var funcDecls []GeminiFunctionDeclaration
		for _, t := range req.Tools {
			fnName, _ := t.Function["name"].(string)
			fnDesc, _ := t.Function["description"].(string)
			fnParams := t.Function["parameters"]
			funcDecls = append(funcDecls, GeminiFunctionDeclaration{
				Name:        fnName,
				Description: fnDesc,
				Parameters:  fnParams,
			})
		}
		if len(funcDecls) > 0 {
			toolContainers = append(toolContainers, GeminiToolDeclarationContainer{
				FunctionDeclarations: funcDecls,
			})
		}
	}

	// Merge consecutive same-role contents to strictly adhere to Gemini's alternating turns contract
	var mergedContents []GeminiContent
	for _, c := range contents {
		if len(mergedContents) > 0 && mergedContents[len(mergedContents)-1].Role == c.Role {
			mergedContents[len(mergedContents)-1].Parts = append(mergedContents[len(mergedContents)-1].Parts, c.Parts...)
		} else {
			mergedContents = append(mergedContents, c)
		}
	}

	geminiReq := &GeminiRequest{
		Contents: mergedContents,
		Tools:    toolContainers,
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
		},
		// Permissive safety settings by default so enterprise and technical code aren't falsely blocked
		SafetySettings: []GeminiSafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
		},
	}

	if len(systemParts) > 0 {
		geminiReq.SystemInstruction = &GeminiSystemInstruction{Parts: systemParts}
	}

	return geminiReq
}
