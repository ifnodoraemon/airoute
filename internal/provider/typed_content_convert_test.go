package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// F-01 regression, outbound half: a typed []ContentPart image (what the protocol
// adapters put in ChatMessage.Content) must be converted into a real image part
// rather than leaked into a text block.
func typedImageContent() []model.ContentPart {
	return []model.ContentPart{
		{Type: model.ContentPartText, Text: "describe this"},
		{Type: model.ContentPartImageURL, ImageURL: &model.ImageURLPart{URL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="}},
	}
}

func typedImageChatRequest() *model.ChatCompletionRequest {
	return &model.ChatCompletionRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []model.ChatMessage{{Role: "user", Content: typedImageContent()}},
	}
}

func TestConvertGeminiSendsTypedImageAsInlineData(t *testing.T) {
	geminiReq := convertOpenAIToGemini(typedImageChatRequest())
	if len(geminiReq.Contents) != 1 {
		t.Fatalf("expected one content turn, got %d", len(geminiReq.Contents))
	}
	var inline, leakedText int
	for _, part := range geminiReq.Contents[0].Parts {
		if part.InlineData != nil {
			inline++
			if part.InlineData.Data != "iVBORw0KGgoAAAANSUhEUg==" {
				t.Fatalf("inlineData lost the base64 payload: %#v", part.InlineData)
			}
		}
		if strings.Contains(part.Text, "base64,") {
			leakedText++
		}
	}
	if inline != 1 || leakedText != 0 {
		t.Fatalf("typed image not converted to inlineData (inline=%d leakedText=%d)", inline, leakedText)
	}
}

func TestConvertAnthropicSendsTypedImageAsImageBlock(t *testing.T) {
	encoded, err := json.Marshal(convertOpenAIToAnthropic(typedImageChatRequest(), &model.ChannelConfig{Name: "claude"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"type":"image"`) {
		t.Fatalf("typed image did not become an image block: %s", string(encoded))
	}
	if strings.Contains(string(encoded), `"text":"[{\"type\"`) {
		t.Fatalf("typed content leaked into a JSON text block: %s", string(encoded))
	}
}

// F-04 regression: OpenAI tool_choice values map onto Anthropic's tool_choice and
// Gemini's toolConfig. Unknown values are dropped (not silently coerced to auto).
func TestAnthropicToolChoiceMapping(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, nil},
		{"auto", "auto", map[string]any{"type": "auto"}},
		{"required", "required", map[string]any{"type": "any"}},
		{"any", "any", map[string]any{"type": "any"}},
		{"none", "none", nil},
		{"function", map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}, map[string]any{"type": "tool", "name": "get_weather"}},
		{"function without name", map[string]any{"type": "function", "function": map[string]any{}}, nil},
		{"unknown string", "sometimes", nil},
		{"unknown object", map[string]any{"type": "mystery"}, nil},
	}
	for _, tc := range cases {
		got := anthropicToolChoice(tc.in)
		if !jsonEqual(got, tc.want) {
			t.Fatalf("%s: anthropicToolChoice(%v) = %#v, want %#v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestGeminiToolConfigMapping(t *testing.T) {
	cases := []struct {
		name     string
		in       any
		wantMode string
		wantTool string
		wantNil  bool
	}{
		{"nil", nil, "", "", true},
		{"auto", "auto", "AUTO", "", false},
		{"required", "required", "ANY", "", false},
		{"none", "none", "NONE", "", false},
		{"function", map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}, "ANY", "get_weather", false},
		{"unknown", "sometimes", "", "", true},
	}
	for _, tc := range cases {
		got := geminiToolConfig(tc.in)
		if tc.wantNil {
			if got != nil {
				t.Fatalf("%s: expected nil toolConfig, got %#v", tc.name, got)
			}
			continue
		}
		if got == nil || got.FunctionCallingConfig == nil {
			t.Fatalf("%s: missing functionCallingConfig: %#v", tc.name, got)
		}
		if got.FunctionCallingConfig.Mode != tc.wantMode {
			t.Fatalf("%s: mode = %q, want %q", tc.name, got.FunctionCallingConfig.Mode, tc.wantMode)
		}
		if tc.wantTool != "" {
			if len(got.FunctionCallingConfig.AllowedFunctionNames) != 1 || got.FunctionCallingConfig.AllowedFunctionNames[0] != tc.wantTool {
				t.Fatalf("%s: allowedFunctionNames = %v, want [%s]", tc.name, got.FunctionCallingConfig.AllowedFunctionNames, tc.wantTool)
			}
		}
	}
}

// The conversion path (not just the helper) must carry the choice into the
// payload, otherwise the forced tool call is still lost on the wire.
func TestConvertersForwardToolChoice(t *testing.T) {
	req := &model.ChatCompletionRequest{
		Model:      "claude-3-5-sonnet",
		Messages:   []model.ChatMessage{{Role: "user", Content: "weather in Paris?"}},
		Tools:      []model.Tool{{Type: "function", Function: map[string]any{"name": "get_weather", "parameters": map[string]any{"type": "object"}}}},
		ToolChoice: "required",
	}

	anthropicReq := convertOpenAIToAnthropic(req, &model.ChannelConfig{Name: "claude"})
	if !jsonEqual(anthropicReq.ToolChoice, map[string]any{"type": "any"}) {
		t.Fatalf("anthropic tool_choice = %#v, want {type: any}", anthropicReq.ToolChoice)
	}
	if len(anthropicReq.Tools) != 1 {
		t.Fatalf("tool declaration not forwarded")
	}

	encoded, err := json.Marshal(convertOpenAIToGemini(req))
	if err != nil {
		t.Fatalf("marshal gemini: %v", err)
	}
	if !strings.Contains(string(encoded), `"toolConfig":{"functionCallingConfig":{"mode":"ANY"}}`) {
		t.Fatalf("gemini toolConfig missing: %s", string(encoded))
	}
}

// jsonEqual compares two JSON-marshalable values by their canonical encoding.
func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}
