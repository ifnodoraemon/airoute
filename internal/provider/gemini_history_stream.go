package provider

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
)

type geminiStreamHistory struct {
	ref   string
	parts []GeminiPart
	calls []model.ToolCall
}

func preserveGeminiStream(ctx context.Context, ch *model.ChannelConfig, name string, response *GeminiResponse, choices []model.ChunkChoice, states map[int]*geminiStreamHistory) error {
	if nativestate.Owner(ctx) == "" {
		return nil
	}
	for i, candidate := range response.Candidates {
		if i >= len(choices) {
			break
		}
		index := choices[i].Index
		state := states[index]
		if state == nil {
			state = &geminiStreamHistory{ref: nativestate.NewID()}
			states[index] = state
		}
		state.parts = append(state.parts, candidate.Content.Parts...)
		tools := choices[i].Delta.ToolCalls
		for j := range tools {
			original := tools[j]
			tools[j].ID = model.GeminiCallPrefix + state.ref + "_" + strconv.Itoa(len(state.calls))
			state.calls = append(state.calls, original)
		}
		if len(tools) > 0 || candidate.FinishReason != "" {
			if err := saveGeminiHistory(ctx, ch, name, state.ref, state.parts, state.calls); err != nil {
				return err
			}
		}
		if candidate.FinishReason != "" {
			if choices[i].Delta.Extra == nil {
				choices[i].Delta.Extra = make(model.ExtraFields)
			}
			choices[i].Delta.Extra["extra_content"], _ = json.Marshal(map[string]any{"google": map[string]string{"state_ref": state.ref}})
		}
	}
	return nil
}
