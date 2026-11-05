package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

func TestAnthropicConversionRejectsThinkingThatWouldBeLost(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	channel := &model.ChannelConfig{Type: model.ProviderAnthropic, BaseURL: server.URL}
	adapter := NewAnthropicProvider(server.Client())
	for _, request := range []*model.ChatCompletionRequest{
		{Extra: model.ExtraFields{"reasoning_effort": json.RawMessage(`"high"`)}},
		{PreparedThinking: &model.PreparedThinking{Control: model.ThinkingControl{Mode: "on", Effort: "high"}, Profile: model.ThinkingProfile{Wire: "glm"}}},
	} {
		_, err := adapter.ChatComplete(context.Background(), request, channel)
		var unsupported *UnsupportedTranslationError
		require.ErrorAs(t, err, &unsupported)
		_, err = adapter.ChatCompleteStream(context.Background(), request, channel)
		require.ErrorAs(t, err, &unsupported)
	}
	require.Zero(t, calls.Load(), "an unrepresentable thinking request must fail before contacting the upstream")
}
