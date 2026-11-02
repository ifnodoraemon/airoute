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
)

func TestNativeProtocolsPreserveUnknownFieldsAndEvents(t *testing.T) {
	cases := []struct {
		protocol, body, reply, stream string
		kind                          model.ProviderType
	}{
		{model.NativeChat, `{"model":"public","messages":[{"role":"user","content":"hi"}],"vendor_option":9007199254740993}`, `{"id":"c","model":"up","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":8,"completion_tokens":3},"vendor_extra":9007199254740993}`, "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"vendor_extra\":9007199254740993}\n\ndata: [DONE]\n\n", model.ProviderOpenAI},
		{model.NativeResponses, `{"model":"public","input":"hi","tools":[{"type":"web_search"}],"vendor_option":9007199254740993}`, `{"id":"r","object":"response","model":"up","status":"completed","output":[],"usage":{"input_tokens":8,"output_tokens":3},"vendor_extra":9007199254740993}`, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"up\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":8,\"output_tokens\":3},\"vendor_extra\":9007199254740993}}\n\n", model.ProviderOpenAI},
		{model.NativeMessages, `{"model":"public","messages":[{"role":"user","content":"hi"}],"max_tokens":20,"vendor_option":9007199254740993}`, `{"id":"a","model":"up","content":[],"usage":{"input_tokens":8,"output_tokens":3},"vendor_extra":9007199254740993}`, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"},\"vendor_extra\":9007199254740993}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", model.ProviderAnthropic},
		{model.NativeGemini, `{"contents":[{"role":"user","parts":[{"text":"hi","thoughtSignature":"opaque"}]}],"vendor_option":9007199254740993}`, `{"candidates":[],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3},"vendor_extra":9007199254740993}`, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\",\"thoughtSignature\":\"opaque\"}]},\"finishReason\":\"STOP\"}],\"vendor_extra\":9007199254740993}\n\n", model.ProviderGemini},
	}
	for _, tc := range cases {
		t.Run(tc.protocol, func(t *testing.T) {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprint(stream), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						if !strings.Contains(string(body), `"vendor_option":9007199254740993`) {
							t.Error("unknown field or integer precision lost")
						}
						if tc.protocol != model.NativeGemini && !strings.Contains(string(body), `"model":"up"`) {
							t.Errorf("model alias wasn't applied: %s", body)
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, tc.stream)
						} else {
							fmt.Fprint(w, tc.reply)
						}
					}))
					defer server.Close()
					ch := &model.ChannelConfig{Name: "a", Type: tc.kind, BaseURL: server.URL, APIKey: "upstream-key", ModelMapping: map[string]string{"public": "up"}}
					req := &model.ChatCompletionRequest{Model: "public", Stream: stream, Native: &model.NativeRequest{Protocol: tc.protocol, Body: json.RawMessage(tc.body)}}
					var p Provider = NewOpenAIProvider(server.Client())
					if tc.kind == model.ProviderAnthropic {
						p = NewAnthropicProvider(server.Client())
					}
					if tc.kind == model.ProviderGemini {
						p = NewGeminiProvider(server.Client())
					}
					if !stream {
						response, e := p.ChatComplete(context.Background(), req, ch)
						if e != nil {
							t.Fatal(e)
						}
						if response.Usage == nil || response.Usage.PromptTokens != 8 || response.Usage.CompletionTokens != 3 {
							t.Fatalf("lost native usage: %+v", response.Usage)
						}
						if !strings.Contains(string(response.NativeResponse), `"vendor_extra":9007199254740993`) {
							t.Fatal("native response fields were rebuilt")
						}
						return
					}
					events, e := p.ChatCompleteStream(context.Background(), req, ch)
					if e != nil {
						t.Fatal(e)
					}
					var wire strings.Builder
					done := false
					for event := range events {
						if event.Err != nil {
							t.Fatal(event.Err)
						}
						wire.Write(event.NativeEvent)
						done = done || event.IsDone
					}
					if !done || !strings.Contains(wire.String(), `"vendor_extra":9007199254740993`) {
						t.Fatalf("lost native event or terminal: %s", wire.String())
					}
				})
			}
		})
	}
}

func TestNativeResponsesTranslatesOnlyForUnsupportedEndpoint(t *testing.T) {
	for _, status := range []int{404, 400, 401, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			chatCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/responses" {
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":{"message":"endpoint not found"}}`)
					return
				}
				chatCalls++
				fmt.Fprint(w, `{"id":"c","choices":[],"usage":{"prompt_tokens":1}}`)
			}))
			defer server.Close()
			req := &model.ChatCompletionRequest{Model: "m", Messages: []model.ChatMessage{{Role: "user", Content: "hi"}}, Native: &model.NativeRequest{Protocol: model.NativeResponses, Body: json.RawMessage(`{"model":"m","input":"hi"}`)}}
			_, err := NewOpenAIProvider(server.Client()).ChatComplete(context.Background(), req, &model.ChannelConfig{Name: "a", BaseURL: server.URL})
			if status == 404 {
				if err != nil || chatCalls != 1 {
					t.Fatalf("unsupported endpoint didn't translate: %v calls=%d", err, chatCalls)
				}
			} else if err == nil || chatCalls != 0 {
				t.Fatalf("status %d caused translation: err=%v calls=%d", status, err, chatCalls)
			}
		})
	}
}
