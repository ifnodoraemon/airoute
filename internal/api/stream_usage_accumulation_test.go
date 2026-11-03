package api

import "testing"

const incrementalUsageSSE = `data: {"id":"u1","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}],"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}

data: {"id":"u1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":50,"total_tokens":50}}

data: [DONE]

`

// TestGeminiStreamUsageAccumulatesAcrossFrames pins the Gemini entrypoint: the
// terminal usageMetadata must keep the prompt count reported earlier.
func TestGeminiStreamUsageAccumulatesAcrossFrames(t *testing.T) {
	body := runGeminiStreamProbe(t, incrementalUsageSSE)
	frames := geminiSSEDataFrames(t, body)

	var usage map[string]any
	for _, frame := range frames {
		if raw, ok := frame["usageMetadata"].(map[string]any); ok {
			usage = raw
		}
	}
	if usage == nil {
		t.Fatalf("no usageMetadata frame reached the client:\n%s", body)
	}
	if got := usage["promptTokenCount"]; got != float64(100) {
		t.Fatalf("promptTokenCount = %v, want 100 (an output-only usage frame erased the earlier prompt count)", got)
	}
	if got := usage["candidatesTokenCount"]; got != float64(50) {
		t.Fatalf("candidatesTokenCount = %v, want 50", got)
	}
	if got := usage["totalTokenCount"]; got != float64(150) {
		t.Fatalf("totalTokenCount = %v, want 150 (prompt+completion, not a single frame's total)", got)
	}
}
