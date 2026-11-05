package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestRelayThinkingEffortReachesOpenAIWireForUnaryAndStream(t *testing.T) {
	var calls atomic.Int32
	efforts := make(chan string, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Model  string `json:"model"`
			Effort string `json:"reasoning_effort"`
			Stream bool   `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || r.URL.Path != "/v1/chat/completions" || request.Model != "aliyun/glm-5.3" || request.Effort == "" {
			t.Errorf("wrong upstream request: path=%s request=%+v", r.URL.Path, request)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Effort == "medium" {
			efforts <- request.Effort
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"unsupported reasoning_effort","type":"invalid_request_error"}}`)
			return
		}
		efforts <- request.Effort
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"thinking-wire\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"391\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, `{"id":"thinking-wire","choices":[{"index":0,"message":{"role":"assistant","content":"391"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	channel := model.ChannelConfig{Name: "relay", Type: model.ProviderOpenAI, BaseURL: upstream.URL, Models: []string{"alias"}, ModelMapping: map[string]string{"alias": "aliyun/glm-5.3"}, Protocols: []string{"openai_chat", "openai_response", "anthropic_messages", "anthropic_native"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"aliyun/glm-5.3": {Thinking: &model.ThinkingConfig{Profile: "glm53"}}}}
	dispatcher := NewDispatcher([]model.ChannelConfig{channel})
	for _, stream := range []bool{false, true} {
		for _, effort := range []string{"low", "high", "max"} {
			t.Run(fmt.Sprintf("%s/stream=%t", effort, stream), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				request := &model.ChatCompletionRequest{Model: "alias", Stream: stream, Messages: []model.ChatMessage{{Role: "user", Content: "17*23"}}, Extra: model.ExtraFields{"thinking_control": json.RawMessage(fmt.Sprintf(`{"mode":"on","effort":%q}`, effort))}}
				if stream {
					events, err := dispatcher.DispatchStream(ctx, request)
					require.NoError(t, err)
					text := ""
					for event := range events {
						require.NoError(t, event.Err)
						if event.Chunk != nil {
							for _, choice := range event.Chunk.Choices {
								text += choice.Delta.Content
							}
						}
					}
					require.Equal(t, "391", text)
				} else {
					response, err := dispatcher.Dispatch(ctx, request)
					require.NoError(t, err)
					require.Equal(t, "391", response.Choices[0].Message.Content)
				}
				require.Equal(t, effort, <-efforts)
			})
		}
	}
	// Legacy SDK parameters must reach the native OpenAI endpoint unchanged, so
	// an upstream rejection cannot turn into success after losing the parameter.
	body := []byte(`{"model":"alias","messages":[{"role":"user","content":"17*23"}],"reasoning_effort":"medium"}`)
	var native model.ChatCompletionRequest
	require.NoError(t, json.Unmarshal(body, &native))
	native.Native = &model.NativeRequest{Protocol: model.NativeChat, Body: body}
	_, err := dispatcher.Dispatch(context.Background(), &native)
	var upstreamError *provider.UpstreamError
	require.ErrorAs(t, err, &upstreamError)
	require.Equal(t, http.StatusBadRequest, upstreamError.Status)
	require.Equal(t, "medium", <-efforts)
	require.EqualValues(t, 7, calls.Load())
	channel.Protocols = []string{"anthropic_native", "anthropic_messages"}
	capability := NewDispatcher([]model.ChannelConfig{channel}).GetThinkingCapabilities([]string{"alias"})["alias"]
	require.False(t, capability.On, "a Messages-only relay cannot advertise OpenAI thinking controls")
}
