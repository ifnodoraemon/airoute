package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

const GeminiHistoryNamespace = "gemini-history"

type geminiHistory struct {
	Parts []GeminiPart     `json:"parts"`
	Calls []model.ToolCall `json:"calls"`
}

func saveGeminiHistory(ctx context.Context, ch *model.ChannelConfig, name, ref string, parts []GeminiPart, calls []model.ToolCall) error {
	owner := nativestate.Owner(ctx)
	if owner == "" {
		return nil
	}
	data, err := json.Marshal(geminiHistory{Parts: parts, Calls: calls})
	if err != nil {
		return err
	}
	return nativestate.Save(ctx, storage.NativeState{Namespace: GeminiHistoryNamespace, ID: ref, Owner: owner,
		Identity: model.UpstreamIdentity(ch), Model: ch.GetUpstreamModel(name), Data: data, ExpiresAt: time.Now().Add(24 * time.Hour)})
}

func preserveGeminiMessage(ctx context.Context, ch *model.ChannelConfig, name string, parts []GeminiPart, message *model.ChatMessage) error {
	if nativestate.Owner(ctx) == "" {
		return nil
	}
	ref := nativestate.NewID()
	if err := saveGeminiHistory(ctx, ch, name, ref, parts, message.ToolCalls); err != nil {
		return err
	}
	for i := range message.ToolCalls {
		message.ToolCalls[i].ID = model.GeminiCallPrefix + ref + "_" + strconv.Itoa(i)
	}
	if message.Extra == nil {
		message.Extra = make(model.ExtraFields)
	}
	message.Extra["extra_content"], _ = json.Marshal(map[string]any{"google": map[string]string{"state_ref": ref}})
	return nil
}

func restoreGeminiHistory(ctx context.Context, req *model.ChatCompletionRequest, ch *model.ChannelConfig) (*model.ChatCompletionRequest, error) {
	copy := *req
	copy.Messages = append([]model.ChatMessage(nil), req.Messages...)
	ids := make(map[string]string)
	for i, msg := range copy.Messages {
		ref := model.GeminiStateReference(msg)
		if ref == "" {
			continue
		}
		saved, err := nativestate.Load(ctx, GeminiHistoryNamespace, ref, nativestate.Owner(ctx))
		if err != nil || saved.Identity != model.UpstreamIdentity(ch) || saved.Model != ch.GetUpstreamModel(req.Model) {
			return nil, &UnsupportedTranslationError{Reason: "Gemini history is unavailable for this caller, upstream or model"}
		}
		var history geminiHistory
		if json.Unmarshal(saved.Data, &history) != nil {
			return nil, fmt.Errorf("invalid stored Gemini history")
		}
		if !geminiHistoryMatches(msg, history, ref) {
			return nil, &UnsupportedTranslationError{Reason: "signed Gemini tool history was modified or truncated"}
		}
		copy.Messages[i].GeminiParts, _ = json.Marshal(history.Parts)
		copy.Messages[i].ToolCalls = append([]model.ToolCall(nil), msg.ToolCalls...)
		for j, call := range copy.Messages[i].ToolCalls {
			original := history.Calls[j].ID
			ids[call.ID] = original
			copy.Messages[i].ToolCalls[j].ID = original
		}
	}
	for i, msg := range copy.Messages {
		if id := ids[msg.ToolCallID]; id != "" {
			copy.Messages[i].ToolCallID = id
		}
	}
	return &copy, nil
}

func geminiHistoryMatches(msg model.ChatMessage, h geminiHistory, ref string) bool {
	if len(msg.ToolCalls) != len(h.Calls) {
		return false
	}
	for i, call := range msg.ToolCalls {
		if call.ID != model.GeminiCallPrefix+ref+"_"+strconv.Itoa(i) || call.Function.Name != h.Calls[i].Function.Name {
			return false
		}
		if !json.Valid([]byte(call.Function.Arguments)) {
			return false
		}
		a, _ := json.Marshal(GeminiFunctionArguments(call.Function.Arguments))
		b, _ := json.Marshal(GeminiFunctionArguments(h.Calls[i].Function.Arguments))
		if string(a) != string(b) {
			return false
		}
	}
	{
		var text strings.Builder
		for _, p := range h.Parts {
			if !p.Thought {
				text.WriteString(p.Text)
			}
		}
		if messageText(msg.Content) != text.String() {
			return false
		}
	}
	return true
}

func messageText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	var text strings.Builder
	for _, p := range model.ParseMessageContent(content) {
		text.WriteString(p.Text)
	}
	return text.String()
}
