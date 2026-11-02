package provider

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func nativeStreamEvent(protocol string, frame []byte, modelName, channel string) *model.StreamEvent {
	event := &model.StreamEvent{NativeProtocol: protocol}
	var data []byte
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("data:")) {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))...)
		}
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		event.IsDone = true
		event.NativeEvent = []byte("data: [DONE]\n\n")
		return event
	}
	if len(data) == 0 {
		event.NativeEvent = append([]byte(nil), frame...)
		return event
	}
	data = RewriteNativeModel(data, modelName)
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		event.Err = err
		return event
	}
	// Retain SSE event/id/retry fields, replacing only its data document.
	var wire bytes.Buffer
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) > 0 && !bytes.HasPrefix(line, []byte("data:")) {
			wire.Write(line)
			wire.WriteByte('\n')
		}
	}
	wire.WriteString("data: ")
	wire.Write(data)
	wire.WriteString("\n\n")
	event.NativeEvent = wire.Bytes()
	populateNativeEvent(event, protocol, object, data)
	event.NativeOutputChars += nativeOutputChars(protocol, object)
	if raw := object["error"]; len(raw) > 0 && string(raw) != "null" {
		event.Err = nativeEventError(channel, raw)
	}
	return event
}

func nativeEventError(channel string, raw []byte) error {
	var failure struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Code    any    `json:"code"`
	}
	_ = json.Unmarshal(raw, &failure)
	kind := failure.Type
	if kind == "" {
		kind = failure.Status
	}
	status := streamErrorStatus(jsonStatusCode(failure.Code), kind)
	if failure.Message == "" {
		failure.Message = string(raw)
	}
	return NewUpstreamError(channel, status, []byte(failure.Message))
}

func populateNativeEvent(event *model.StreamEvent, protocol string, object map[string]json.RawMessage, data []byte) {
	if protocol == model.NativeGeminiInteractions {
		populateInteractionEvent(event, object)
		return
	}
	if protocol == model.NativeChat {
		populateNativeChatEvent(event, data)
		return
	}
	var kind string
	_ = json.Unmarshal(object["type"], &kind)
	usageObject := object
	if protocol == model.NativeResponses && len(object["response"]) > 0 {
		_ = json.Unmarshal(object["response"], &usageObject)
	}
	if protocol == model.NativeMessages && len(object["message"]) > 0 {
		_ = json.Unmarshal(object["message"], &usageObject)
	}
	var id string
	_ = json.Unmarshal(usageObject["id"], &id)
	var geminiUsage *GeminiUsageMetadata
	_ = json.Unmarshal(object["usageMetadata"], &geminiUsage)
	event.Chunk = &model.ChatCompletionChunk{ID: id, Usage: nativeUsage(protocol, usageObject["usage"], geminiUsage)}
	switch protocol {
	case model.NativeResponses:
		event.NativeContent = strings.HasSuffix(kind, ".delta") || kind == "response.output_item.added"
		event.IsDone = kind == "response.completed" || kind == "response.incomplete" || kind == "response.failed"
		if kind == "response.failed" {
			event.Err = nativeEventError("", usageObject["error"])
		}
	case model.NativeMessages:
		event.NativeContent = kind == "content_block_start" || kind == "content_block_delta"
		event.IsDone = kind == "message_stop"
	case model.NativeGemini:
		var candidates []struct {
			Content struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		}
		_ = json.Unmarshal(object["candidates"], &candidates)
		for _, candidate := range candidates {
			event.NativeContent = event.NativeContent || len(candidate.Content.Parts) > 0
			event.IsDone = event.IsDone || candidate.FinishReason != ""
		}
		if feedback := object["promptFeedback"]; len(feedback) > 0 && bytes.Contains(feedback, []byte("blockReason")) {
			event.IsDone = true
		}
	}
	if raw := object["error"]; len(raw) > 0 && string(raw) != "null" && event.Err == nil {
		event.Err = NewUpstreamError("", http.StatusBadGateway, raw)
	}
}

func populateNativeChatEvent(event *model.StreamEvent, data []byte) {
	var chunk model.ChatCompletionChunk
	if json.Unmarshal(data, &chunk) == nil {
		event.Chunk = &chunk
		for _, choice := range chunk.Choices {
			event.NativeOutputChars += len(choice.Delta.Content) + len(choice.Delta.ReasoningContent)
			for _, tool := range choice.Delta.ToolCalls {
				event.NativeOutputChars += len(tool.Function.Arguments)
			}
			event.NativeContent = event.NativeContent || choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" || len(choice.Delta.ToolCalls) > 0
			event.IsDone = event.IsDone || choice.FinishReason != nil
		}
	}
}
