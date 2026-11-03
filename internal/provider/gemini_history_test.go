package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
)

func TestGeminiHistoryCarrierSurvivesDroppedExtensionsAndKeepsWholeParts(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests++
		if requests == 2 {
			for _, want := range []string{`"thoughtSignature":"text-signature=="`, `"thoughtSignature":"image-signature=="`, `"thoughtSignature":"tool-signature=="`, `"opaque":9007199254740993`, `"id":"native-call"`} {
				if !strings.Contains(string(body), want) {
					t.Errorf("history lost %s: %s", want, body)
				}
			}
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"answer","thoughtSignature":"text-signature=="},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="},"thoughtSignature":"image-signature=="},{"functionCall":{"id":"native-call","name":"lookup","args":{"n":9007199254740993}},"thoughtSignature":"tool-signature==","opaque":9007199254740993}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()
	p, ch := NewGeminiProvider(server.Client()), &model.ChannelConfig{BaseURL: server.URL, Type: model.ProviderGemini}
	ctx := nativestate.WithOwner(context.Background(), "history-owner")
	response, err := p.ChatComplete(ctx, &model.ChatCompletionRequest{Model: "m", Messages: []model.ChatMessage{{Role: "user", Content: "first"}}}, ch)
	if err != nil {
		t.Fatal(err)
	}
	message := response.Choices[0].Message
	if len(message.ToolCalls[0].ID) > 40 || !strings.HasPrefix(message.ToolCalls[0].ID, model.GeminiCallPrefix) {
		t.Fatalf("unsafe carrier: %s", message.ToolCalls[0].ID)
	}
	message.Extra = nil
	message.ToolCalls[0].ExtraContent = nil
	request := &model.ChatCompletionRequest{Model: "m", Messages: []model.ChatMessage{message, {Role: "tool", ToolCallID: message.ToolCalls[0].ID, Content: `{"value":1}`}}}
	if _, err := p.ChatComplete(ctx, request, ch); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ChatComplete(nativestate.WithOwner(context.Background(), "foreign-owner"), request, ch); err == nil {
		t.Fatal("foreign caller replayed signed history")
	}
	altered := *request
	altered.Messages = append([]model.ChatMessage(nil), request.Messages...)
	altered.Messages[0].ToolCalls = append([]model.ToolCall(nil), message.ToolCalls...)
	altered.Messages[0].ToolCalls[0].Function.Arguments = `{"n":1}`
	if _, err := p.ChatComplete(ctx, &altered, ch); err == nil {
		t.Fatal("modified signed arguments were accepted")
	}
	if requests != 2 {
		t.Fatalf("invalid history reached upstream %d times", requests)
	}
}

func TestGeminiStreamCarriesSignatureUntilEmptyTextTerminalPart(t *testing.T) {
	ctx := nativestate.WithOwner(context.Background(), "stream-owner")
	ch := &model.ChannelConfig{BaseURL: "https://stream.invalid", Type: model.ProviderGemini}
	states := make(map[int]*geminiStreamHistory)
	converter := &geminiStreamChoices{}
	var message model.ChatMessage
	for _, data := range []string{`{"candidates":[{"content":{"parts":[{"text":"answer"}]}}]}`, `{"candidates":[{"content":{"parts":[{"text":"","thoughtSignature":"empty-final-signature=="}]},"finishReason":"STOP"}]}`} {
		var chunk GeminiResponse
		_ = json.Unmarshal([]byte(data), &chunk)
		choices := converter.convert(&chunk)
		if err := preserveGeminiStream(ctx, ch, "m", &chunk, choices, states); err != nil {
			t.Fatal(err)
		}
		message.Content = message.GetContentString() + choices[0].Delta.Content
		if choices[0].Delta.Extra != nil {
			message.Extra = choices[0].Delta.Extra
		}
	}
	restored, err := restoreGeminiHistory(ctx, &model.ChatCompletionRequest{Model: "m", Messages: []model.ChatMessage{message}}, ch)
	if err != nil || !strings.Contains(string(restored.Messages[0].GeminiParts), "empty-final-signature==") {
		t.Fatalf("empty signed part lost: %v", err)
	}
}
