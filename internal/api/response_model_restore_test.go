package api

import (
	"encoding/json"
	"testing"
)

// B-08: a provider (or its channel mapping) may echo a different model name than
// the client requested. Only the top-level field may change.
func TestRestoreResponseModelJSON(t *testing.T) {
	t.Run("rewrites only the top-level model", func(t *testing.T) {
		body := []byte(`{"id":"img-1","model":"dall-e-3-upstream","data":[{"model":"keep-me"}]}`)
		got := restoreResponseModelJSON(body, "flux-1.1-pro")
		var payload map[string]any
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatalf("restored body is not JSON: %v (%s)", err, got)
		}
		if payload["model"] != "flux-1.1-pro" {
			t.Errorf("top-level model = %v, want flux-1.1-pro", payload["model"])
		}
		nested := payload["data"].([]any)[0].(map[string]any)
		if nested["model"] != "keep-me" {
			t.Errorf("nested model was rewritten: %v", nested["model"])
		}
		if payload["id"] != "img-1" {
			t.Errorf("other fields changed: %v", payload["id"])
		}
	})

	t.Run("leaves a matching model untouched", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o","data":1}`)
		got := restoreResponseModelJSON(body, "gpt-4o")
		if string(got) != string(body) {
			t.Errorf("body changed although the model already matched: %s", got)
		}
	})

	t.Run("passes non-JSON through", func(t *testing.T) {
		body := []byte{0x49, 0x44, 0x33, 0x04, 0x00} // MP3 frame header
		if got := restoreResponseModelJSON(body, "tts-1"); string(got) != string(body) {
			t.Errorf("binary payload was modified")
		}
	})

	t.Run("passes malformed JSON through", func(t *testing.T) {
		body := []byte(`{"model":`)
		if got := restoreResponseModelJSON(body, "m"); string(got) != string(body) {
			t.Errorf("malformed body was modified: %s", got)
		}
	})

	t.Run("passes a JSON array through", func(t *testing.T) {
		body := []byte(`[{"model":"x"}]`)
		if got := restoreResponseModelJSON(body, "m"); string(got) != string(body) {
			t.Errorf("array body was modified: %s", got)
		}
	})

	t.Run("leaves a body without model unchanged", func(t *testing.T) {
		body := []byte(`{"id":"vid_1","status":"PENDING"}`)
		if got := restoreResponseModelJSON(body, "cogvideox"); string(got) != string(body) {
			t.Errorf("body without model was modified: %s", got)
		}
	})
}
