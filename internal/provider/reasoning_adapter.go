package provider

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

const (
	ThinkTagStart = "<think>"
	ThinkTagEnd   = "</think>"
)

// IsReasoningModel determines if a model is known to generate thinking/reasoning traces.
func IsReasoningModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	return strings.Contains(lower, "r1") ||
		strings.Contains(lower, "v4.1") ||
		strings.Contains(lower, "dsv4") ||
		strings.Contains(lower, "reason") ||
		strings.Contains(lower, "think") ||
		strings.Contains(lower, "qwq") ||
		strings.Contains(lower, "o1") ||
		strings.Contains(lower, "o3") ||
		strings.Contains(lower, "o4")
}

// SeparateReasoningInResponse inspects non-streaming ChatCompletionResponse.
// If message.ReasoningContent is empty, but message.Content contains </think> (or begins with <think>),
// it cleanly separates the thinking trace into ReasoningContent and keeps only the final answer in Content.
func SeparateReasoningInResponse(resp *model.ChatCompletionResponse) {
	if resp == nil {
		return
	}
	for i := range resp.Choices {
		msg := &resp.Choices[i].Message
		if msg.ReasoningContent != "" {
			continue // Already separated upstream
		}
		contentStr, ok := msg.Content.(string)
		if !ok || contentStr == "" {
			continue
		}

		if strings.Contains(contentStr, ThinkTagEnd) {
			parts := strings.SplitN(contentStr, ThinkTagEnd, 2)
			rawReasoning := parts[0]
			rawAnswer := parts[1]

			// Strip optional leading <think>
			rawReasoning = strings.TrimPrefix(strings.TrimSpace(rawReasoning), ThinkTagStart)
			msg.ReasoningContent = strings.TrimSpace(rawReasoning)
			msg.Content = strings.TrimLeft(rawAnswer, "\r\n")
		} else if strings.HasPrefix(strings.TrimSpace(contentStr), ThinkTagStart) {
			// In case the model hit max_tokens while still thinking and never emitted </think>
			trimmed := strings.TrimSpace(contentStr)
			msg.ReasoningContent = strings.TrimSpace(strings.TrimPrefix(trimmed, ThinkTagStart))
			msg.Content = ""
		}
	}
}

// ReasoningStreamAdapter adapts a stream of ChatCompletionChunk objects so that
// thinking tokens are emitted in chunk.Delta.ReasoningContent and the final response
// is emitted in chunk.Delta.Content.
type ReasoningStreamAdapter struct {
	modelName        string
	isReasoningModel bool
	inThinking       bool
	sawReasoning     bool
	passedThinkEnd   bool
	started          bool
	buffer           string // Holds partial </think> across chunk boundaries
}

// NewReasoningStreamAdapter creates a new stream processor for the requested model and channel.
func NewReasoningStreamAdapter(modelName string, channel *model.ChannelConfig) *ReasoningStreamAdapter {
	isReasoning := IsReasoningModel(modelName)
	if !isReasoning && channel != nil {
		lowerType := strings.ToLower(string(channel.Type))
		lowerName := strings.ToLower(channel.Name)
		if (lowerType == "gpustack" || strings.Contains(lowerName, "gpustack")) &&
			(strings.Contains(strings.ToLower(modelName), "deepseek") || strings.Contains(strings.ToLower(modelName), "glm")) {
			isReasoning = true
		}
	}

	return &ReasoningStreamAdapter{
		modelName:        modelName,
		isReasoningModel: isReasoning,
		inThinking:       isReasoning,
	}
}

// ProcessChunk processes an incoming ChatCompletionChunk.
// It returns a slice of chunks (0, 1, or 2 chunks) to send downstream.
// When transitioning across </think>, it may split a single chunk into two:
// one terminating the reasoning, and one starting the content.
func (a *ReasoningStreamAdapter) ProcessChunk(chunk *model.ChatCompletionChunk) []*model.ChatCompletionChunk {
	if chunk == nil || len(chunk.Choices) == 0 {
		return []*model.ChatCompletionChunk{chunk}
	}

	delta := &chunk.Choices[0].Delta

	// If upstream explicitly provided reasoning_content, we never interfere!
	if delta.ReasoningContent != "" {
		a.sawReasoning = true
		return []*model.ChatCompletionChunk{chunk}
	}
	if a.sawReasoning {
		return []*model.ChatCompletionChunk{chunk}
	}

	// If this chunk does not carry content (e.g. role-only, finish_reason, or usage), pass through
	if delta.Content == "" {
		return []*model.ChatCompletionChunk{chunk}
	}

	rawText := delta.Content

	// On the very first text token:
	if !a.started {
		a.started = true
		// If text starts with <think>, force thinking mode even if model name didn't match
		if strings.HasPrefix(strings.TrimSpace(rawText), ThinkTagStart) {
			a.inThinking = true
			rawText = strings.TrimPrefix(strings.TrimSpace(rawText), ThinkTagStart)
		}
	}

	// If not in thinking mode:
	if !a.inThinking {
		// Just in case a non-reasoning-flagged model suddenly emits <think> mid-stream
		if strings.Contains(rawText, ThinkTagStart) {
			a.inThinking = true
			idx := strings.Index(rawText, ThinkTagStart)
			contentBefore := rawText[:idx]
			textAfter := rawText[idx+len(ThinkTagStart):]

			var result []*model.ChatCompletionChunk
			if contentBefore != "" {
				c1 := deepCopyChunk(chunk)
				c1.Choices[0].Delta.Content = contentBefore
				c1.Choices[0].Delta.ReasoningContent = ""
				result = append(result, c1)
			}

			remainingChunks := a.processThinkingText(chunk, textAfter)
			result = append(result, remainingChunks...)
			return result
		}

		// Normal content pass through
		return []*model.ChatCompletionChunk{chunk}
	}

	// We are currently in thinking mode!
	return a.processThinkingText(chunk, rawText)
}

func (a *ReasoningStreamAdapter) processThinkingText(baseChunk *model.ChatCompletionChunk, incomingText string) []*model.ChatCompletionChunk {
	combined := a.buffer + incomingText
	a.buffer = ""

	// Strip leading <think> if any
	combined = strings.TrimPrefix(combined, ThinkTagStart)

	// Check if combined contains </think>
	if idx := strings.Index(combined, ThinkTagEnd); idx != -1 {
		reasoningPart := combined[:idx]
		contentPart := combined[idx+len(ThinkTagEnd):]

		a.inThinking = false
		a.passedThinkEnd = true

		var result []*model.ChatCompletionChunk

		// 1. Emit reasoning chunk if there is text before </think>
		if reasoningPart != "" {
			cReason := deepCopyChunk(baseChunk)
			cReason.Choices[0].Delta.ReasoningContent = reasoningPart
			cReason.Choices[0].Delta.Content = ""
			result = append(result, cReason)
		}

		// 2. Emit content chunk if there is text after </think>
		contentPart = strings.TrimLeft(contentPart, "\r\n")
		if contentPart != "" {
			cContent := deepCopyChunk(baseChunk)
			cContent.Choices[0].Delta.Content = contentPart
			cContent.Choices[0].Delta.ReasoningContent = ""
			result = append(result, cContent)
		}

		return result
	}

	// Does combined end with a partial prefix of </think>?
	// Check prefixes of "</think>" (lengths 1 to 7)
	suffixLen := matchThinkEndPrefix(combined)
	if suffixLen > 0 {
		a.buffer = combined[len(combined)-suffixLen:]
		combined = combined[:len(combined)-suffixLen]
	}

	if combined == "" {
		return nil // All characters buffered for pending </think> match
	}

	c := deepCopyChunk(baseChunk)
	c.Choices[0].Delta.ReasoningContent = combined
	c.Choices[0].Delta.Content = ""
	return []*model.ChatCompletionChunk{c}
}

// Flush returns any residual buffered bytes upon stream completion.
func (a *ReasoningStreamAdapter) Flush(baseChunk *model.ChatCompletionChunk) []*model.ChatCompletionChunk {
	if a.buffer == "" {
		return nil
	}
	remaining := a.buffer
	a.buffer = ""

	if baseChunk == nil {
		return nil
	}
	c := deepCopyChunk(baseChunk)
	if a.inThinking {
		c.Choices[0].Delta.ReasoningContent = remaining
		c.Choices[0].Delta.Content = ""
	} else {
		c.Choices[0].Delta.Content = remaining
		c.Choices[0].Delta.ReasoningContent = ""
	}
	return []*model.ChatCompletionChunk{c}
}

func matchThinkEndPrefix(s string) int {
	for l := 7; l >= 1; l-- {
		if len(s) >= l {
			prefix := ThinkTagEnd[:l]
			if strings.HasSuffix(s, prefix) {
				return l
			}
		}
	}
	return 0
}

func deepCopyChunk(src *model.ChatCompletionChunk) *model.ChatCompletionChunk {
	if src == nil {
		return nil
	}
	dst := *src
	if len(src.Choices) > 0 {
		dst.Choices = make([]model.ChunkChoice, len(src.Choices))
		copy(dst.Choices, src.Choices)
	}
	return &dst
}
