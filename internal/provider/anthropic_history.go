package provider

import (
	"encoding/json"
	"github.com/ifnodoraemon/airoute/internal/model"
	"sort"
)

// Retain the signed provider blocks verbatim alongside the display text. A
// reasoning_content string alone cannot reconstruct an Anthropic tool history.
type anthropicHistoryState struct {
	blocks map[int]map[string]json.RawMessage
	inputs map[int]string
}

func (s *anthropicHistoryState) Observe(raw []byte) {
	var event struct {
		Type  string                     `json:"type"`
		Index int                        `json:"index"`
		Block map[string]json.RawMessage `json:"content_block"`
		Delta map[string]json.RawMessage `json:"delta"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	if s.blocks == nil {
		s.blocks = make(map[int]map[string]json.RawMessage)
		s.inputs = make(map[int]string)
	}
	if event.Type == "content_block_start" {
		s.blocks[event.Index] = event.Block
		return
	}
	if event.Type != "content_block_delta" {
		return
	}
	block := s.blocks[event.Index]
	if block == nil {
		return
	}
	for _, field := range []string{"text", "thinking", "signature"} {
		var value string
		if json.Unmarshal(event.Delta[field], &value) != nil {
			continue
		}
		var previous string
		_ = json.Unmarshal(block[field], &previous)
		block[field], _ = json.Marshal(previous + value)
	}
	var partial string
	if json.Unmarshal(event.Delta["partial_json"], &partial) == nil {
		s.inputs[event.Index] += partial
		if json.Valid([]byte(s.inputs[event.Index])) {
			block["input"] = json.RawMessage(s.inputs[event.Index])
		}
	}
}
func anthropicHistoryExtra(content json.RawMessage) model.ExtraFields {
	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	signed := false
	for _, block := range blocks {
		if block.Type == "thinking" || block.Type == "redacted_thinking" {
			signed = true
		}
	}
	if !signed {
		return nil
	}
	raw, _ := json.Marshal(map[string]any{"anthropic": map[string]any{"content": content}})
	return model.ExtraFields{"extra_content": raw}
}
func (s *anthropicHistoryState) Extra() model.ExtraFields {
	indices := make([]int, 0, len(s.blocks))
	for index := range s.blocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	blocks := make([]map[string]json.RawMessage, 0, len(indices))
	for _, index := range indices {
		blocks = append(blocks, s.blocks[index])
	}
	content, _ := json.Marshal(blocks)
	return anthropicHistoryExtra(content)
}
func restoredAnthropicHistory(msg model.ChatMessage) json.RawMessage {
	var extra struct {
		Anthropic struct {
			Content json.RawMessage `json:"content"`
		} `json:"anthropic"`
	}
	if json.Unmarshal(msg.Extra["extra_content"], &extra) != nil || anthropicHistoryExtra(extra.Anthropic.Content) == nil {
		return nil
	}
	return extra.Anthropic.Content
}
