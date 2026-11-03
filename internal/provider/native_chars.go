package provider

import (
	"encoding/json"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// Only generated text/arguments count toward a missing-usage estimate. Binary
// media, signatures, ids and repeated terminal snapshots are never text tokens.
func nativeOutputChars(protocol string, object map[string]json.RawMessage) int {
	var delta struct {
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
		JSON     string `json:"partial_json"`
	}
	switch protocol {
	case model.NativeResponses:
		var kind, value string
		_ = json.Unmarshal(object["type"], &kind)
		if kind == "response.output_text.delta" || kind == "response.function_call_arguments.delta" || kind == "response.reasoning_summary_text.delta" || kind == "response.reasoning_text.delta" {
			_ = json.Unmarshal(object["delta"], &value)
		}
		return len(value)
	case model.NativeMessages:
		_ = json.Unmarshal(object["delta"], &delta)
		return len(delta.Text) + len(delta.Thinking) + len(delta.JSON)
	case model.NativeGemini:
		var candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
					Call *struct {
						Args json.RawMessage `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		}
		_ = json.Unmarshal(object["candidates"], &candidates)
		total := 0
		for _, candidate := range candidates {
			for _, part := range candidate.Content.Parts {
				total += len(part.Text)
				if part.Call != nil {
					total += len(part.Call.Args)
				}
			}
		}
		return total
	}
	return 0
}

func nativeUnaryOutputChars(protocol string, body []byte) int {
	var object map[string]json.RawMessage
	_ = json.Unmarshal(body, &object)
	if protocol == model.NativeGemini {
		return nativeOutputChars(protocol, object)
	}
	var blocks []struct {
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		Input     json.RawMessage `json:"input"`
		Arguments string          `json:"arguments"`
		Content   []struct {
			Text string `json:"text"`
		} `json:"content"`
		Summary []struct {
			Text string `json:"text"`
		} `json:"summary"`
	}
	raw := object["content"]
	if protocol == model.NativeResponses {
		raw = object["output"]
	}
	_ = json.Unmarshal(raw, &blocks)
	total := 0
	for _, block := range blocks {
		total += len(block.Text) + len(block.Thinking) + len(block.Arguments) + len(block.Input)
		for _, part := range block.Content {
			total += len(part.Text)
		}
		for _, part := range block.Summary {
			total += len(part.Text)
		}
	}
	return total
}
