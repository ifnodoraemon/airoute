package model

import (
	"encoding/json"
	"strings"
)

const GeminiCallPrefix = "call_ag_"

func GeminiStateReference(message ChatMessage) string {
	for _, call := range message.ToolCalls {
		if strings.HasPrefix(call.ID, GeminiCallPrefix) {
			ref, _, _ := strings.Cut(strings.TrimPrefix(call.ID, GeminiCallPrefix), "_")
			return ref
		}
	}
	var extra struct {
		Google struct {
			Ref string `json:"state_ref"`
		} `json:"google"`
	}
	_ = json.Unmarshal(message.Extra["extra_content"], &extra)
	return extra.Google.Ref
}
