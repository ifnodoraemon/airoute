package controlplane

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestThinkingProbeReusesStoredCredentialWithoutReturningIt(t *testing.T) {
	handler, repo := newChannelTestHandler(t)
	prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
		if body["reasoning_effort"] == invalidProbeEffort {
			return 400, `{"error":{"message":"reasoning_effort invalid; supported values are none, low, high, ultra"}}`
		}
		if body["reasoning_effort"] == "none" {
			return 200, probeCompletion(0)
		}
		return 200, probeCompletion(12)
	})
	handler.prober = prober
	record := &storage.ChannelRecord{Name: "upstream", Type: model.ProviderOpenAI, BaseURL: channel.BaseURL, APIKey: channel.APIKey, Models: []string{"physical-model"}}
	require.NoError(t, repo.CreateChannel(record))
	body := fmt.Sprintf(`{"channel_id":%d,"base_url":%q,"type":"openai","model":"physical-model"}`, record.ID, channel.BaseURL)
	response := invokeChannel(t, handler.ProbeChannelThinking, http.MethodPost, body, "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"ultra"`)
	require.NotContains(t, response.Body.String(), channel.APIKey)
	require.Nil(t, storedChannel(t, repo, record.ID).ModelCapabilities)
}

func TestThinkingProbeDoesNotSendStoredCredentialToEditedURL(t *testing.T) {
	handler, repo := newChannelTestHandler(t)
	var outbound atomic.Int32
	handler.prober = NewDownstreamProber(&http.Client{Transport: thinkingProbeTransport(func(*http.Request) (*http.Response, error) {
		outbound.Add(1)
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})})
	record := &storage.ChannelRecord{Name: "upstream", Type: model.ProviderOpenAI, BaseURL: "https://original.example/v1", APIKey: "private-key"}
	require.NoError(t, repo.CreateChannel(record))
	body := fmt.Sprintf(`{"channel_id":%d,"base_url":"https://different.example/v1","type":"openai","model":"physical-model"}`, record.ID)
	response := invokeChannel(t, handler.ProbeChannelThinking, http.MethodPost, body, "")
	require.Equal(t, 400, response.Code)
	require.Contains(t, response.Body.String(), "服务地址已变更")
	require.NotContains(t, response.Body.String(), record.APIKey)
	require.Zero(t, outbound.Load())
}

func TestThinkingProbeRejectsUnsafeAndAmbiguousTargets(t *testing.T) {
	handler, _ := newChannelTestHandler(t)
	for _, body := range []string{
		`{"base_url":"http://169.254.169.254","model":"physical-model"}`,
		`{"base_url":"https://upstream.example","model":"*"}`,
		`{"base_url":"https://upstream.example","model":"physical-model","thinking":{"efforts":["none"]}}`,
	} {
		response := invokeChannel(t, handler.ProbeChannelThinking, http.MethodPost, body, "")
		require.Equal(t, 400, response.Code, body)
	}
}
