package model

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// ThinkingObservation stores configuration and reported evidence, never thought
// text. A missing reasoning count remains nil rather than being reported as 0.
type ThinkingObservation struct {
	Mode            string `json:"mode"`
	Effort          string `json:"effort,omitempty"`
	IncludeSummary  *bool  `json:"include_summary,omitempty"`
	Source          string `json:"source"`
	Profile         string `json:"profile,omitempty"`
	UpstreamModel   string `json:"upstream_model"`
	ReasoningTokens *int   `json:"reasoning_tokens,omitempty"`
	HasSummary      bool   `json:"has_summary"`
	FinishReason    string `json:"finish_reason,omitempty"`
}
type ThinkingTrace struct {
	mu          sync.Mutex
	observation *ThinkingObservation
	emitted     bool
}
type thinkingTraceKey struct{}

func WithThinkingTrace(ctx context.Context) context.Context {
	if ThinkingTraceFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, thinkingTraceKey{}, &ThinkingTrace{})
}
func ThinkingTraceFromContext(ctx context.Context) *ThinkingTrace {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(thinkingTraceKey{}).(*ThinkingTrace)
	return v
}
func (t *ThinkingTrace) Set(observation ThinkingObservation) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observation = &observation
	t.emitted = false
}
func (t *ThinkingTrace) Snapshot() *ThinkingObservation {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.observation == nil {
		return nil
	}
	copy := *t.observation
	if copy.ReasoningTokens != nil {
		v := *copy.ReasoningTokens
		copy.ReasoningTokens = &v
	}
	return &copy
}
func (t *ThinkingTrace) Observe(usage *Usage, reasoning, finish string, native []byte) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.observation == nil {
		return
	}
	if count := usage.ReportedReasoningTokens(); count != nil {
		t.observation.ReasoningTokens = count
	}

	if reasoning != "" {
		t.observation.HasSummary = true
	}
	if finish != "" {
		t.observation.FinishReason = finish
	}
	if len(native) > 0 {
		var body struct {
			Usage struct {
				Thoughts *int `json:"thoughtsTokenCount"`
			} `json:"usageMetadata"`
			Candidates []struct {
				Finish  string `json:"finishReason"`
				Content struct {
					Parts []struct {
						Thought bool   `json:"thought"`
						Text    string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal(native, &body) == nil {
			if body.Usage.Thoughts != nil {
				t.observation.ReasoningTokens = body.Usage.Thoughts
			}
			for _, candidate := range body.Candidates {
				if candidate.Finish != "" {
					t.observation.FinishReason = candidate.Finish
				}
				for _, part := range candidate.Content.Parts {
					if part.Thought && part.Text != "" {
						t.observation.HasSummary = true
					}
				}
			}
		}
	}
}
func (t *ThinkingTrace) ObserveEvent(event *StreamEvent) {
	if t == nil || event == nil {
		return
	}
	if event.Chunk != nil {
		reasoning, finish := "", ""
		for _, choice := range event.Chunk.Choices {
			reasoning += choice.Delta.ReasoningContent
			if strings.Contains(choice.Delta.Content, "<|channel>thought") {
				reasoning += "thought"
			}
			if choice.FinishReason != nil {
				finish = *choice.FinishReason
			}
		}
		t.Observe(event.Chunk.Usage, reasoning, finish, event.NativeEvent)
		terminal := event.Chunk.Usage != nil
		for _, choice := range event.Chunk.Choices {
			terminal = terminal || choice.FinishReason != nil
		}
		if !t.shouldPublish(terminal) {
			return
		}
		if snapshot := t.Snapshot(); snapshot != nil {
			if event.Chunk.Extra == nil {
				event.Chunk.Extra = make(ExtraFields)
			}
			event.Chunk.Extra["airoute_thinking"], _ = json.Marshal(snapshot)
		}
	} else {
		t.Observe(nil, "", "", event.NativeEvent)
	}
}

func (t *ThinkingTrace) shouldPublish(terminal bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.emitted && !terminal {
		return false
	}
	t.emitted = true
	return true
}
