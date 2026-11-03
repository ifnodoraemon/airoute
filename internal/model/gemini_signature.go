package model

import "encoding/json"

// ThoughtSignature follows Google's Chat Completions compatibility convention.
// The opaque signature must survive a tool result round trip byte for byte.
func (t ToolCall) ThoughtSignature() string {
	var extra struct {
		Google struct {
			Signature string `json:"thought_signature"`
		} `json:"google"`
	}
	_ = json.Unmarshal(t.ExtraContent, &extra)
	return extra.Google.Signature
}

func (t *ToolCall) SetThoughtSignature(signature string) {
	if signature == "" {
		return
	}
	var extra map[string]json.RawMessage
	_ = json.Unmarshal(t.ExtraContent, &extra)
	if extra == nil {
		extra = make(map[string]json.RawMessage)
	}
	var google map[string]json.RawMessage
	_ = json.Unmarshal(extra["google"], &google)
	if google == nil {
		google = make(map[string]json.RawMessage)
	}
	google["thought_signature"], _ = json.Marshal(signature)
	extra["google"], _ = json.Marshal(google)
	t.ExtraContent, _ = json.Marshal(extra)
}
