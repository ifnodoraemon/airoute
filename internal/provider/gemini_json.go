package provider

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Gemini uses ProtoJSON, whose field names accept both lowerCamelCase and the
// original snake_case. Normalize only a parsing copy, keeping user schemas,
// function arguments/results and the native request bytes unchanged.
func CanonicalGeminiJSON(body []byte) []byte {
	if !bytes.Contains(body, []byte("_")) {
		return body
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return body
	}
	canonicalGeminiFields(value)
	out, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return out
}

func canonicalGeminiFields(value any) {
	switch object := value.(type) {
	case []any:
		for _, item := range object {
			canonicalGeminiFields(item)
		}
	case map[string]any:
		for key, item := range object {
			name := geminiCamelField(key)
			if key != name {
				delete(object, key)
				if _, exists := object[name]; exists {
					continue
				}
				object[name] = item
			}
			switch name {
			case "args", "response", "parameters", "parametersJsonSchema", "responseSchema", "responseJsonSchema", "labels":
				continue
			}
			canonicalGeminiFields(item)
		}
	}
}

func geminiCamelField(name string) string {
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func (call *GeminiFunctionCall) UnmarshalJSON(body []byte) error {
	type wireCall GeminiFunctionCall
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return decoder.Decode((*wireCall)(call))
}

func GeminiFunctionArguments(raw string) map[string]any {
	var args map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&args)
	if args == nil {
		args = map[string]any{}
	}
	return args
}

func (part *GeminiPart) UnmarshalJSON(data []byte) error {
	type fields GeminiPart
	if err := json.Unmarshal(data, (*fields)(part)); err != nil {
		return err
	}
	part.Raw = append([]byte(nil), data...)
	return nil
}

func (part GeminiPart) MarshalJSON() ([]byte, error) {
	if len(part.Raw) > 0 {
		return part.Raw, nil
	}
	type fields GeminiPart
	return json.Marshal(fields(part))
}
