package provider

import "github.com/ifnodoraemon/airoute/internal/model"

// buildAnthropicDeltaChunk renders an Anthropic content_block_delta into an
// OpenAI-shaped chunk (text, extended thinking, or tool-call argument fragment).
// Extracted from ChatCompleteStream to keep the streaming function under the size
// red line. Returns nil when the delta carries nothing renderable. toolIndexFor
// is called only for a tool-argument fragment because it advances a counter.
func buildAnthropicDeltaChunk(messageID string, created int64, modelName string, delta map[string]any, blockIdx int, toolIndexFor func(int) int) *model.ChatCompletionChunk {
	base := func(d model.ChunkDelta) *model.ChatCompletionChunk {
		return &model.ChatCompletionChunk{
			ID:      messageID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   modelName,
			Choices: []model.ChunkChoice{{Index: 0, Delta: d}},
		}
	}
	if text, ok := delta["text"].(string); ok && text != "" {
		return base(model.ChunkDelta{Content: text})
	}
	// Extended thinking (thinking_delta) used to fall through and vanish on the
	// adapted path; keep it as reasoning_content so OpenAI-shaped clients see the
	// trace the upstream billed for.
	if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
		return base(model.ChunkDelta{ReasoningContent: thinking})
	}
	if partial, ok := delta["partial_json"].(string); ok {
		idx := toolIndexFor(blockIdx)
		return base(model.ChunkDelta{ToolCalls: []model.ToolCall{
			{Index: &idx, Type: "function", Function: model.FunctionCall{Arguments: partial}},
		}})
	}
	return nil
}

// buildAnthropicToolStartChunk renders a content_block_start tool_use block into
// an OpenAI-shaped chunk announcing the tool call (id + name, no arguments yet).
// Returns nil for non-tool_use blocks. toolIndexFor is called only for a tool_use
// block because it advances a counter.
func buildAnthropicToolStartChunk(messageID string, created int64, modelName string, cb map[string]any, blockIdx int, toolIndexFor func(int) int) *model.ChatCompletionChunk {
	if cb["type"] != "tool_use" {
		return nil
	}
	toolID, _ := cb["id"].(string)
	toolName, _ := cb["name"].(string)
	idx := toolIndexFor(blockIdx)
	return &model.ChatCompletionChunk{
		ID:      messageID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   modelName,
		Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{
			ToolCalls: []model.ToolCall{
				{Index: &idx, ID: toolID, Type: "function", Function: model.FunctionCall{Name: toolName}},
			},
		}}},
	}
}

// buildAnthropicRoleChunk renders the initial message_start chunk that announces
// the assistant role.
func buildAnthropicRoleChunk(messageID string, created int64, modelName string) *model.ChatCompletionChunk {
	return &model.ChatCompletionChunk{
		ID:      messageID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   modelName,
		Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{Role: "assistant"}}},
	}
}