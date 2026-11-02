package provider

import (
	"context"
	"fmt"
	"github.com/ifnodoraemon/airoute/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiSeparatesThoughtPartsInUnaryAndMultilineSSE(t *testing.T) {
	body := `{"candidates":[{"content":{"parts":[{"text":"private reasoning","thought":true},{"text":"final answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"thoughtsTokenCount":3}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "sse" {
			fmt.Fprint(w, "data: {\ndata: \"candidates\":[{\"content\":{\"parts\":[{\"text\":\"private reasoning\",\"thought\":true},{\"text\":\"final answer\"}]},\"finishReason\":\"STOP\"}],\ndata: \"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":2,\"thoughtsTokenCount\":3}\ndata: }")
		} else {
			fmt.Fprint(w, body)
		}
	}))
	defer server.Close()
	p := NewGeminiProvider(server.Client())
	ch := &model.ChannelConfig{Name: "gemma", BaseURL: server.URL}
	response, e := p.ChatComplete(context.Background(), &model.ChatCompletionRequest{Model: "gemma-4-31b-it"}, ch)
	if e != nil {
		t.Fatal(e)
	}
	if response.Choices[0].Message.GetContentString() != "final answer" || response.Choices[0].Message.ReasoningContent != "private reasoning" {
		t.Fatalf("thought part mixed into answer: %+v", response.Choices)
	}
	events, e := p.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "gemma-4-31b-it", Stream: true}, ch)
	if e != nil {
		t.Fatal(e)
	}
	var answer, thought string
	for event := range events {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Chunk != nil {
			for _, choice := range event.Chunk.Choices {
				answer += choice.Delta.Content
				thought += choice.Delta.ReasoningContent
			}
		}
	}
	if answer != "final answer" || thought != "private reasoning" {
		t.Fatalf("multiline SSE lost content: answer=%q thought=%q", answer, thought)
	}
}
