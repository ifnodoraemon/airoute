package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestGeminiTranslationPreservesGenerationOptionsAndToolHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		var config GeminiGenerationConfig
		_ = json.Unmarshal(request["generationConfig"], &config)
		if config.MaxOutputTokens == nil || *config.MaxOutputTokens != 129 || len(config.StopSequences) != 2 || config.CandidateCount == nil || *config.CandidateCount != 2 {
			t.Errorf("generation options lost: %s", request["generationConfig"])
		}
		if config.ResponseMIMEType != "application/json" || len(config.ResponseJSONSchema) == 0 || config.Seed == nil || *config.Seed != 9007199254740993 {
			t.Errorf("JSON schema or seed lost: %s", request["generationConfig"])
		}
		if config.ThinkingConfig == nil || config.ThinkingConfig.ThinkingBudget == nil || *config.ThinkingConfig.ThinkingBudget != 8192 {
			t.Errorf("thinking must use mapped upstream model: %s", request["generationConfig"])
		}
		var contents []GeminiContent
		_ = json.Unmarshal(request["contents"], &contents)
		part := contents[0].Parts[0]
		if part.ThoughtSignature != "signed==" || part.FunctionCall.ID != "call-original" || part.FunctionCall.Args["exact_id"] != json.Number("9007199254740993") || contents[1].Parts[0].FunctionResponse.ID != "call-original" {
			t.Errorf("tool history lost: %s", request["contents"])
		}
		if _, exists := request["safetySettings"]; exists {
			t.Error("unsolicited safety override")
		}
		fmt.Fprint(w, `{"candidates":[{"index":0,"content":{"parts":[{"text":"one"}]},"finishReason":"STOP"},{"index":1,"content":{"parts":[{"functionCall":{"id":"native-id","name":"lookup","args":{}},"thoughtSignature":"returned=="}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()
	n := 2
	call := model.ToolCall{ID: "call-original", Type: "function", Function: model.FunctionCall{Name: "lookup", Arguments: `{"exact_id":9007199254740993}`}}
	call.SetThoughtSignature("signed==")
	request := &model.ChatCompletionRequest{Model: "alias", N: &n, Stop: []any{"END", "STOP"}, Messages: []model.ChatMessage{
		{Role: "assistant", ToolCalls: []model.ToolCall{call}}, {Role: "tool", ToolCallID: call.ID, Content: `{}`},
	}, Extra: model.ExtraFields{
		"max_completion_tokens": json.RawMessage(`129`), "reasoning_effort": json.RawMessage(`"medium"`),
		"seed": json.RawMessage(`9007199254740993`), "response_format": json.RawMessage(`{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}`),
	}}
	channel := &model.ChannelConfig{Name: "google", BaseURL: server.URL, ModelMapping: map[string]string{"alias": "gemini-2.5-flash"}}
	response, err := NewGeminiProvider(server.Client()).ChatComplete(context.Background(), request, channel)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Choices) != 2 || response.Choices[1].Index != 1 {
		t.Fatalf("candidate lost: %+v", response.Choices)
	}
	returned := response.Choices[1].Message.ToolCalls[0]
	if returned.ID != "native-id" || returned.ThoughtSignature() != "returned==" {
		t.Fatalf("tool state lost: %+v", returned)
	}
}

func TestGeminiStreamKeepsParallelCallsDistinctAndWaitsForFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"first\",\"args\":{}},\"thoughtSignature\":\"sig==\"}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"second\",\"args\":{}}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"},{\"index\":1,\"content\":{\"parts\":[{\"text\":\"other choice\"}]},\"finishReason\":\"MAX_TOKENS\"}],\"usageMetadata\":{\"promptTokenCount\":7,\"candidatesTokenCount\":9}}\n\n")
	}))
	defer server.Close()
	stream, err := NewGeminiProvider(server.Client()).ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, &model.ChannelConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	calls, completed, other := 0, false, false
	for event := range stream {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Chunk == nil {
			continue
		}
		for _, choice := range event.Chunk.Choices {
			for _, call := range choice.Delta.ToolCalls {
				if choice.FinishReason != nil {
					t.Fatal("tool call prematurely ended stream")
				}
				if call.Index == nil || *call.Index != calls {
					t.Fatalf("parallel call index lost: %+v", call)
				}
				if calls == 0 && call.ThoughtSignature() != "sig==" {
					t.Fatal("signature lost")
				}
				calls++
			}
			if choice.FinishReason != nil && choice.Index == 0 {
				completed = *choice.FinishReason == "tool_calls"
			}
			if choice.Index == 1 {
				other = choice.Delta.Content == "other choice" && *choice.FinishReason == "length"
			}
		}
	}
	if calls != 2 || !completed || !other {
		t.Fatalf("stream lost state: calls=%d completed=%v other=%v", calls, completed, other)
	}
}

func TestGeminiBlockedPromptIsContentFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":7}}`
		if r.URL.Query().Get("alt") == "sse" {
			fmt.Fprintf(w, "data: %s\n\n", body)
		} else {
			fmt.Fprint(w, body)
		}
	}))
	defer server.Close()
	p, channel := NewGeminiProvider(server.Client()), &model.ChannelConfig{BaseURL: server.URL}
	response, err := p.ChatComplete(context.Background(), &model.ChatCompletionRequest{Model: "m"}, channel)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Choices) != 1 || *response.Choices[0].FinishReason != "content_filter" {
		t.Fatalf("blocked prompt lost: %+v", response)
	}
	stream, err := p.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, channel)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for event := range stream {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Chunk != nil && len(event.Chunk.Choices) > 0 {
			found = *event.Chunk.Choices[0].FinishReason == "content_filter"
		}
	}
	if !found {
		t.Fatal("blocked prompt stream lost terminal state")
	}
}
