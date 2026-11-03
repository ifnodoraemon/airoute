package provider

import (
	"context"
	"encoding/json"
	"io"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func GeminiInteractionUsage(raw []byte) *model.Usage {
	var usage struct {
		Input   int `json:"total_input_tokens"`
		Output  int `json:"total_output_tokens"`
		Thought int `json:"total_thought_tokens"`
		Cached  int `json:"total_cached_tokens"`
		Total   int `json:"total_tokens"`
	}
	if json.Unmarshal(raw, &usage) != nil {
		return nil
	}
	output := usage.Output + usage.Thought
	return &model.Usage{PromptTokens: usage.Input, CompletionTokens: output,
		TotalTokens: max(usage.Total, usage.Input+output), PromptTokensDetails: cacheTokenDetails(usage.Cached, 0)}
}

func populateInteractionEvent(event *model.StreamEvent, object map[string]json.RawMessage) {
	var kind string
	_ = json.Unmarshal(object["event_type"], &kind)
	var interaction struct {
		ID     string          `json:"id"`
		Usage  json.RawMessage `json:"usage"`
		Status string          `json:"status"`
		Error  json.RawMessage `json:"error"`
	}
	_ = json.Unmarshal(object["interaction"], &interaction)
	event.Chunk = &model.ChatCompletionChunk{ID: interaction.ID, Usage: GeminiInteractionUsage(interaction.Usage)}
	if len(interaction.Usage) == 0 {
		var metadata struct {
			Usage json.RawMessage `json:"total_usage"`
		}
		_ = json.Unmarshal(object["metadata"], &metadata)
		event.Chunk.Usage = GeminiInteractionUsage(metadata.Usage)
	}
	event.IsDone = kind == "interaction.completed" || kind == "interaction.complete"
	event.NativeContent = kind == "content.delta" || kind == "step.delta" || kind == "step.start"
	if interaction.Status == "failed" {
		event.Err = nativeEventError("", interaction.Error)
	}
	var delta struct {
		Text  string `json:"text"`
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
	}
	_ = json.Unmarshal(object["delta"], &delta)
	event.NativeOutputChars = len(delta.Text) + len(delta.Delta.Text)
}

func ReadNativeProtocol(ctx context.Context, body io.ReadCloser, protocol, name string, ch *model.ChannelConfig) <-chan *model.StreamEvent {
	request := &model.ChatCompletionRequest{Model: name, Native: &model.NativeRequest{Protocol: protocol}}
	out := make(chan *model.StreamEvent, 64)
	go readNativeStream(ctx, out, body, request, ch)
	return out
}
