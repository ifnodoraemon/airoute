package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// Gemini-only features stay eligible for native dispatch. A fallback must not
// silently discard grounding, cached context, signed parts, or media metadata.
func geminiTranslationError(body []byte) error {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if err := geminiKnownFields(body, "request", "contents systemInstruction generationConfig tools toolConfig thinking_control"); err != nil {
		return err
	}
	if err := geminiKnownFields(fields["generationConfig"], "generationConfig", "temperature topP maxOutputTokens stopSequences candidateCount presencePenalty frequencyPenalty seed responseMimeType responseJsonSchema responseSchema thinkingConfig"); err != nil {
		return err
	}
	var config struct {
		MIME     string          `json:"responseMimeType"`
		Thinking json.RawMessage `json:"thinkingConfig"`
	}
	_ = json.Unmarshal(fields["generationConfig"], &config)
	if config.MIME != "" && config.MIME != "application/json" && config.MIME != "text/plain" {
		return fmt.Errorf("responseMimeType %q requires a native Gemini upstream", config.MIME)
	}
	if err := geminiKnownFields(config.Thinking, "thinkingConfig", "thinkingLevel"); err != nil {
		return err
	}
	var contents []struct {
		Parts []json.RawMessage `json:"parts"`
	}
	_ = json.Unmarshal(fields["contents"], &contents)
	for _, content := range contents {
		for _, part := range content.Parts {
			if err := geminiPartTranslation(part); err != nil {
				return err
			}
		}
	}
	return geminiToolTranslation(fields["tools"], fields["toolConfig"])
}

func geminiKnownFields(raw []byte, path, names string) error {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for name := range fields {
		if !strings.Contains(" "+names+" ", " "+name+" ") {
			return fmt.Errorf("%s.%s requires a native Gemini upstream", path, name)
		}
	}
	return nil
}

func geminiPartTranslation(raw []byte) error {
	if err := geminiKnownFields(raw, "part", "text inlineData fileData functionCall functionResponse"); err != nil {
		return err
	}
	var part map[string]json.RawMessage
	_ = json.Unmarshal(raw, &part)
	for _, key := range []string{"inlineData", "fileData"} {
		var media struct {
			MIME string `json:"mimeType"`
		}
		_ = json.Unmarshal(part[key], &media)
		if len(part[key]) > 0 && !strings.HasPrefix(media.MIME, "image/") {
			return fmt.Errorf("%s with MIME type %q requires a native Gemini upstream", key, media.MIME)
		}
	}
	if err := geminiKnownFields(part["functionCall"], "functionCall", "id name args"); err != nil {
		return err
	}
	return geminiKnownFields(part["functionResponse"], "functionResponse", "id name response")
}

func geminiToolTranslation(raw, config []byte) error {
	var tools []map[string]json.RawMessage
	_ = json.Unmarshal(raw, &tools)
	for _, tool := range tools {
		for key := range tool {
			if key != "functionDeclarations" {
				return fmt.Errorf("gemini tool %q requires a native Gemini upstream", key)
			}
		}
		var declarations []json.RawMessage
		_ = json.Unmarshal(tool["functionDeclarations"], &declarations)
		for _, declaration := range declarations {
			if err := geminiKnownFields(declaration, "functionDeclaration", "name description parameters parametersJsonSchema"); err != nil {
				return err
			}
		}
	}
	if err := geminiKnownFields(config, "toolConfig", "functionCallingConfig"); err != nil {
		return err
	}
	var options struct {
		Function struct {
			Mode  string   `json:"mode"`
			Names []string `json:"allowedFunctionNames"`
		} `json:"functionCallingConfig"`
	}
	_ = json.Unmarshal(config, &options)
	if len(options.Function.Names) > 1 || (options.Function.Mode != "" && options.Function.Mode != "AUTO" && options.Function.Mode != "ANY" && options.Function.Mode != "NONE") {
		return fmt.Errorf("functionCallingConfig requires a native Gemini upstream")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(config, &fields)
	return geminiKnownFields(fields["functionCallingConfig"], "functionCallingConfig", "mode allowedFunctionNames")
}

func geminiEmbeddingTranslationError(body []byte) error {
	body = provider.CanonicalGeminiJSON(body)
	if err := geminiKnownFields(body, "embedContent", "model content outputDimensionality"); err != nil {
		return err
	}
	var request struct {
		Content struct {
			Parts []json.RawMessage `json:"parts"`
		} `json:"content"`
	}
	_ = json.Unmarshal(body, &request)
	for _, part := range request.Content.Parts {
		if err := geminiKnownFields(part, "embedding part", "text"); err != nil {
			return err
		}
	}
	return nil
}

func applyGeminiTranslation(native *model.NativeRequest) {
	native.TranslationError = geminiTranslationError(provider.CanonicalGeminiJSON(native.Body))
}
