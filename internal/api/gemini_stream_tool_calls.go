package api

import (
	"encoding/json"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// geminiStreamToolCall is one tool call being assembled across stream deltas.
type geminiStreamToolCall struct {
	name string
	args strings.Builder
}

// geminiStreamToolCalls assembles OpenAI-style streaming tool_call fragments into
// complete Gemini functionCall parts.
//
// OpenAI streams a tool call as fragments keyed by tool_call.index: the first
// fragment carries the id and the function name, later fragments carry slices of
// the JSON arguments string (a single call routinely spans 3+ frames). Gemini has
// no equivalent: one functionCall part must carry the complete name and a
// complete args object.
//
// Translating each fragment on its own — the previous behaviour — produced one
// functionCall per frame: the first with a name and args=null, the rest with an
// empty name and args=null, and the assembled arguments never reached the client
// (BUG-GEMINI-TOOLCALL).
type geminiStreamToolCalls struct {
	order []int
	calls map[int]*geminiStreamToolCall
	// lastIndex is the call a fragment without an explicit index continues.
	// -1 means "no call opened yet".
	lastIndex int
}

func newGeminiStreamToolCalls() *geminiStreamToolCalls {
	return &geminiStreamToolCalls{
		calls:     make(map[int]*geminiStreamToolCall),
		lastIndex: -1,
	}
}

// add folds one upstream tool_call delta into the accumulator.
//
// The index is taken from the delta when the upstream provides one (the normal
// case, and what makes parallel calls distinguishable). Without an index the
// delta is ordered by appearance: a fragment that carries an id or a name starts
// the next call, a bare arguments fragment continues the most recent call.
func (g *geminiStreamToolCalls) add(tc model.ToolCall) {
	idx := -1
	if tc.Index != nil {
		idx = *tc.Index
	} else {
		startsNewCall := tc.ID != "" || tc.Function.Name != ""
		if !startsNewCall && g.lastIndex >= 0 {
			idx = g.lastIndex
		} else {
			idx = g.nextIndex()
		}
	}
	if idx < 0 {
		idx = 0
	}
	call, ok := g.calls[idx]
	if !ok {
		call = &geminiStreamToolCall{}
		g.calls[idx] = call
		g.order = append(g.order, idx)
	}
	if call.name == "" && tc.Function.Name != "" {
		call.name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		call.args.WriteString(tc.Function.Arguments)
	}
	g.lastIndex = idx
}

// nextIndex returns the smallest unused ordinal.
func (g *geminiStreamToolCalls) nextIndex() int {
	for i := 0; ; i++ {
		if _, used := g.calls[i]; !used {
			return i
		}
	}
}

// pending reports whether any call is still waiting to be emitted.
func (g *geminiStreamToolCalls) pending() bool { return len(g.order) > 0 }

// drainFunctionCalls returns the assembled calls in first-seen order and resets
// the accumulator, so one call set is never emitted twice. Gemini's
// functionCall.args is an object, so an empty or unparsable argument string
// becomes {} instead of the null a nil map would encode to.
func (g *geminiStreamToolCalls) drainFunctionCalls() []geminiFunctionCall {
	if len(g.order) == 0 {
		return nil
	}
	out := make([]geminiFunctionCall, 0, len(g.order))
	for _, idx := range g.order {
		call := g.calls[idx]
		if call == nil {
			continue
		}
		var args map[string]any
		if raw := strings.TrimSpace(call.args.String()); raw != "" {
			_ = json.Unmarshal([]byte(raw), &args)
		}
		if args == nil {
			args = map[string]any{}
		}
		out = append(out, geminiFunctionCall{Name: call.name, Args: args})
	}
	g.order = nil
	g.calls = make(map[int]*geminiStreamToolCall)
	g.lastIndex = -1
	return out
}

// geminiFunctionCall is the transport-neutral shape of a Gemini functionCall part.
type geminiFunctionCall struct {
	Name string
	Args map[string]any
}
