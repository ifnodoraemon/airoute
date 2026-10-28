package provider

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

const (
	ThinkTagStart = "<think>"
	ThinkTagEnd   = "</think>"
)

// SeparateReasoningInResponse inspects non-streaming ChatCompletionResponse.
// If message.ReasoningContent is empty, but message.Content contains </think> (or begins with <think>),
// it cleanly separates the thinking trace into ReasoningContent and keeps only the final answer in Content.
// Completely model-agnostic: driven purely by standard thinking tags.
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

// ReasoningStreamAdapter adapts a stream of ChatCompletionChunk objects based purely on
// <think> and </think> tags. Zero hardcoded model names.
type ReasoningStreamAdapter struct {
	inThinking     bool
	sawReasoning   bool
	passedThinkEnd bool
	buffer         string // Holds partial tags (<think> or </think>) across chunk boundaries
}

// NewReasoningStreamAdapter creates a new tag-driven stream processor.
func NewReasoningStreamAdapter() *ReasoningStreamAdapter {
	return &ReasoningStreamAdapter{}
}

// ProcessChunk processes an incoming ChatCompletionChunk.
// If the stream contains <think> ... </think>, it routes thinking tokens to
// chunk.Delta.ReasoningContent and answer tokens to chunk.Delta.Content.
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

	// If this chunk does not carry text content, pass through
	if delta.Content == "" {
		return []*model.ChatCompletionChunk{chunk}
	}

	rawText := delta.Content

	// If not currently in thinking mode:
	if !a.inThinking {
		// If we already passed </think>, all subsequent tokens are strictly content
		if a.passedThinkEnd {
			return []*model.ChatCompletionChunk{chunk}
		}

		combined := a.buffer + rawText
		a.buffer = ""

		// Check if combined contains <think>
		if idx := strings.Index(combined, ThinkTagStart); idx != -1 {
			a.inThinking = true
			contentBefore := combined[:idx]
			textAfter := combined[idx+len(ThinkTagStart):]

			var result []*model.ChatCompletionChunk
			if contentBefore != "" {
				c1 := deepCopyChunk(chunk)
				c1.Choices[0].Delta.Content = contentBefore
				c1.Choices[0].Delta.ReasoningContent = ""
				result = append(result, c1)
			}

			// Process remainder in thinking mode
			remainingChunks := a.processThinkingText(chunk, textAfter)
			result = append(result, remainingChunks...)
			return result
		}

		// Check if combined ends with a partial prefix of <think>
		if suffixLen := matchTagPrefix(combined, ThinkTagStart); suffixLen > 0 {
			a.buffer = combined[len(combined)-suffixLen:]
			combined = combined[:len(combined)-suffixLen]
		}

		if combined == "" {
			return nil
		}

		c := deepCopyChunk(chunk)
		c.Choices[0].Delta.Content = combined
		c.Choices[0].Delta.ReasoningContent = ""
		return []*model.ChatCompletionChunk{c}
	}

	// Currently in thinking mode
	return a.processThinkingText(chunk, rawText)
}

func (a *ReasoningStreamAdapter) processThinkingText(baseChunk *model.ChatCompletionChunk, incomingText string) []*model.ChatCompletionChunk {
	combined := a.buffer + incomingText
	a.buffer = ""

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

	// Check if combined ends with a partial prefix of </think>
	if suffixLen := matchTagPrefix(combined, ThinkTagEnd); suffixLen > 0 {
		a.buffer = combined[len(combined)-suffixLen:]
		combined = combined[:len(combined)-suffixLen]
	}

	if combined == "" {
		return nil
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
	if len(c.Choices) == 0 {
		// The last frame before [DONE] is usually a usage-only chunk
		// (stream_options.include_usage) with an empty choices array. Indexing
		// Choices[0] here used to panic inside the provider goroutine and take
		// the whole process down. Rebuild a synthetic choice so the buffered
		// tail is not silently dropped either.
		c.Choices = []model.ChunkChoice{{Index: 0}}
		// Never duplicate usage: the original frame carries the authoritative
		// numbers and will be forwarded by the caller.
		c.Usage = nil
	}
	if a.inThinking {
		c.Choices[0].Delta.ReasoningContent = remaining
		c.Choices[0].Delta.Content = ""
	} else {
		c.Choices[0].Delta.Content = remaining
		c.Choices[0].Delta.ReasoningContent = ""
	}
	return []*model.ChatCompletionChunk{c}
}

func matchTagPrefix(s string, tag string) int {
	maxLen := len(tag) - 1
	for l := maxLen; l >= 1; l-- {
		if len(s) >= l {
			prefix := tag[:l]
			if strings.HasSuffix(s, prefix) {
				return l
			}
		}
	}
	return 0
}

// streamChunkSender delivers one adapted chunk downstream. Returning false means
// the client went away and the stream must stop.
type streamChunkSender func(*model.ChatCompletionChunk) bool

// reasoningStreamPipeline is the streaming counterpart of
// SeparateReasoningInResponse: it applies the same inline <think>...</think>
// split to every chunk and flushes a trailing partial tag at EOF.
//
// Both OpenAI-compatible streaming paths use it (chat completions and the legacy
// text-completion adapter). Keeping one implementation is what makes the unary
// and streaming rules identical: the text path used to emit its raw text without
// any reasoning split, so the same upstream reported thinking tokens inline when
// streamed and as reasoning_content when not (B-12).
type reasoningStreamPipeline struct {
	adapter *ReasoningStreamAdapter
	last    *model.ChatCompletionChunk
}

func newReasoningStreamPipeline() *reasoningStreamPipeline {
	return &reasoningStreamPipeline{adapter: NewReasoningStreamAdapter()}
}

// Emit adapts one upstream chunk and forwards the resulting downstream chunks.
func (p *reasoningStreamPipeline) Emit(chunk *model.ChatCompletionChunk, send streamChunkSender) bool {
	for _, processed := range p.adapter.ProcessChunk(chunk) {
		if processed == nil {
			continue
		}
		p.last = processed
		if !send(processed) {
			return false
		}
	}
	return true
}

// Flush forwards text still held back by a partial tag.
func (p *reasoningStreamPipeline) Flush(send streamChunkSender) bool {
	if p.last == nil {
		return true
	}
	for _, flushed := range p.adapter.Flush(p.last) {
		if !send(flushed) {
			return false
		}
	}
	return true
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
