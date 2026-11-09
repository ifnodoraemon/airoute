package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

type thinkingProbeTransport func(*http.Request) (*http.Response, error)

func (transport thinkingProbeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func thinkingProbeFixture(t *testing.T, respond func(map[string]any) (int, string)) (*DownstreamProber, *model.ChannelConfig) {
	t.Helper()
	allowLoopbackProbe(t)
	client := &http.Client{Transport: thinkingProbeTransport(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/chat/completions", request.URL.Path)
		require.Equal(t, "Bearer upstream-secret", request.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, "physical-model", body["model"])
		require.NotContains(t, body, "thinking_control")
		status, response := respond(body)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	return NewDownstreamProber(client), &model.ChannelConfig{Type: model.ProviderOpenAI, BaseURL: "https://upstream.example/v1", APIKey: "upstream-secret"}
}

func probeCompletion(tokens int) string {
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": "391"}}}, "usage": map[string]any{"completion_tokens_details": map[string]int{"reasoning_tokens": tokens}}})
	return string(data)
}

func TestThinkingProbeDiscoversOffUltraAndCustomLevel(t *testing.T) {
	prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
		require.EqualValues(t, 128, body["max_completion_tokens"])
		switch body["reasoning_effort"] {
		case "none":
			return http.StatusOK, probeCompletion(0)
		case "low", "high", "ultra", "extended":
			return http.StatusOK, probeCompletion(12)
		default:
			return http.StatusBadRequest, `{"error":{"param":"reasoning_effort","message":"Invalid effort. Supported values: none, low, high, ultra, extended"}}`
		}
	})
	channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{"physical-model": {Thinking: &model.ThinkingConfig{Efforts: []string{"extended"}}}}
	result := prober.ProbeThinking(context.Background(), channel, "physical-model")
	require.NotContains(t, result.Checks, "minimal")
	require.NotNil(t, result.Thinking)
	require.True(t, *result.Thinking.CanDisable)
	require.Equal(t, []string{"low", "high", "ultra", "extended"}, result.Thinking.Efforts)
	require.Equal(t, "unsupported", result.Checks["max"].Status)
	require.Equal(t, "supported", result.Checks["ultra"].Status)
	require.Equal(t, "probe", result.Capability.Source)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "upstream-secret")
	require.Equal(t, []string{"extended"}, channel.ModelCapabilities["physical-model"].Thinking.Efforts)
}

func TestThinkingProbeDoesNotTrustIgnoredFields(t *testing.T) {
	prober, channel := thinkingProbeFixture(t, func(map[string]any) (int, string) {
		return http.StatusOK, probeCompletion(0)
	})
	result := prober.ProbeThinking(context.Background(), channel, "physical-model")
	require.Nil(t, result.Thinking)
	require.False(t, result.Capability.Known)
	for option, check := range result.Checks {
		require.Equal(t, "unknown", check.Status, option)
	}
}

func TestThinkingProbeRecognizesRelayEnumErrorWithoutEchoedValue(t *testing.T) {
	for _, response := range []string{
		`{"error":{"code":"invalid_parameter_error","param":null,"message":"'reasoning_effort' must be one of: 'low', 'high', 'max'","type":"invalid_request_error"}}`,
		`{"error":{"message":"'reasoning_effort' must be one of: 'low', 'high', 'max'"}}`,
	} {
		prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
			if body["reasoning_effort"] == "low" || body["reasoning_effort"] == "high" || body["reasoning_effort"] == "max" {
				return 200, probeCompletion(12)
			}
			return 400, response
		})
		result := prober.ProbeThinking(context.Background(), channel, "physical-model")
		require.NotNil(t, result.Thinking)
		require.Equal(t, []string{"low", "high", "max"}, result.Thinking.Efforts)
		for _, effort := range []string{"low", "high", "max"} {
			require.Equal(t, "supported", result.Checks[effort].Status, effort)
		}
		for _, effort := range []string{"off", "medium", "xhigh", "ultra"} {
			require.Equal(t, "unsupported", result.Checks[effort].Status, effort)
		}
	}
}

func TestThinkingProbeRechecksFixedAndUnknownPresets(t *testing.T) {
	for _, preset := range []string{"reasoning-only", "unsupported", "unknown"} {
		prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
			switch body["reasoning_effort"] {
			case "none":
				return 200, probeCompletion(0)
			case "low", "high", "max":
				return 200, probeCompletion(12)
			default:
				return 400, `{"error":{"message":"reasoning_effort must be one of low, high, max, none"}}`
			}
		})
		channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{"physical-model": {Thinking: &model.ThinkingConfig{Profile: preset}}}
		result := prober.ProbeThinking(context.Background(), channel, "physical-model")
		require.Empty(t, result.Message)
		require.Equal(t, "supported", result.Checks["off"].Status)
		require.Equal(t, []string{"low", "high", "max"}, result.Thinking.Efforts)
		require.Equal(t, "openai-toggle", result.Thinking.Profile)
		require.Equal(t, preset, channel.ModelCapabilities["physical-model"].Thinking.Profile)
	}
}

func TestThinkingProbeDiscoversDeepSeekDialectWithoutAModelNameAllowlist(t *testing.T) {
	prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
		require.EqualValues(t, 128, body["max_tokens"])
		require.NotContains(t, body, "max_completion_tokens")
		if body["thinking"].(map[string]any)["type"] == "disabled" {
			return 200, probeCompletion(0)
		}
		require.Equal(t, "enabled", body["thinking"].(map[string]any)["type"])
		if body["reasoning_effort"] == invalidProbeEffort {
			return 400, `{"error":{"param":"reasoning_effort","message":"value __airoute_invalid_effort__ is invalid"}}`
		}
		return 200, probeCompletion(17)
	})
	channel.Type = model.ProviderDeepSeek
	channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{"physical-model": {Thinking: &model.ThinkingConfig{Profile: "reasoning-only", Efforts: []string{}}}}
	result := prober.ProbeThinking(context.Background(), channel, "physical-model")
	require.Equal(t, "deepseek", result.Thinking.Profile)
	require.True(t, *result.Thinking.CanDisable)
	require.Equal(t, model.ThinkingEfforts(), result.Thinking.Efforts)
	for _, check := range result.Checks {
		require.Equal(t, "supported", check.Status)
	}
}

func TestThinkingProbeChangedDialectDoesNotInventUnconfirmedLevels(t *testing.T) {
	prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
		if body["reasoning_effort"] == "none" {
			return 200, probeCompletion(0)
		}
		return 200, probeCompletion(12)
	})
	channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{"physical-model": {Thinking: &model.ThinkingConfig{Profile: "reasoning-only"}}}
	result := prober.ProbeThinking(context.Background(), channel, "physical-model")
	require.Equal(t, "openai-toggle", result.Thinking.Profile)
	require.True(t, *result.Thinking.CanDisable)
	require.NotNil(t, result.Thinking.Efforts)
	require.Empty(t, result.Thinking.Efforts)
	require.Contains(t, result.Message, "未确认可调档位")
	for _, effort := range model.ThinkingEfforts() {
		require.Equal(t, "unknown", result.Checks[effort].Status)
	}
}

func TestThinkingProbeRetainsKnownLevelsWhenOnlyDisableWasConfirmed(t *testing.T) {
	prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
		if body["thinking"] != nil {
			return 400, `{"error":{"message":"thinking disabled is not supported"}}`
		}
		return 200, probeCompletion(12)
	})
	channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{"physical-model": {Thinking: &model.ThinkingConfig{Profile: "glm53"}}}
	result := prober.ProbeThinking(context.Background(), channel, "physical-model")
	require.NotNil(t, result.Thinking.CanDisable)
	require.False(t, *result.Thinking.CanDisable)
	require.Nil(t, result.Thinking.Efforts)
	require.True(t, result.Capability.On)
	require.Equal(t, []string{"low", "high", "max"}, result.Capability.Efforts)
	require.Equal(t, "unknown", result.Checks["low"].Status)
}

func TestThinkingProbeKeepsTransientFailuresUnconfirmed(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			prober, channel := thinkingProbeFixture(t, func(map[string]any) (int, string) {
				return status, `{"error":{"message":"reasoning_effort temporarily unavailable"}}`
			})
			result := prober.ProbeThinking(context.Background(), channel, "physical-model")
			require.Nil(t, result.Thinking)
			for option, check := range result.Checks {
				require.Equal(t, "unknown", check.Status, option)
			}
		})
	}
}

func TestThinkingProbeRejectsIgnoredDisable(t *testing.T) {
	prober, channel := thinkingProbeFixture(t, func(body map[string]any) (int, string) {
		if body["reasoning_effort"] == invalidProbeEffort {
			return 400, `{"error":{"message":"reasoning_effort invalid; supported values are low, high"}}`
		}
		return 200, probeCompletion(12)
	})
	result := prober.ProbeThinking(context.Background(), channel, "physical-model")
	require.NotNil(t, result.Thinking.CanDisable)
	require.False(t, *result.Thinking.CanDisable)
	require.Equal(t, "unsupported", result.Checks["off"].Status)
}

func TestThinkingMetadataUsesExplicitLevelsAndKeepsCatalogFallback(t *testing.T) {
	var entries []thinkingModelMetadata
	require.NoError(t, json.Unmarshal([]byte(`[{"id":"custom","capabilities":{"reasoning_effort":["none","minimal","low","ultra","extended"]}},{"id":"gpt-6-astra"},{"id":"broken","capabilities":{"reasoning_effort":["bogus value"]}}]`), &entries))
	result := &ProbeResult{Type: model.ProviderCustom, Models: []string{"custom", "gpt-6-astra", "broken"}, ThinkingCapabilities: metadataThinkingCapabilities(model.ProviderCustom, entries)}
	completeProbeThinking(result)
	custom := result.ThinkingCapabilities["custom"]
	require.True(t, custom.Off)
	require.Equal(t, []string{"low", "ultra", "extended"}, custom.Efforts)
	require.Equal(t, "metadata", custom.Source)
	require.NotContains(t, result.ThinkingCapabilities["gpt-6-astra"].Efforts, "ultra")
	require.Equal(t, "catalog", result.ThinkingCapabilities["gpt-6-astra"].Source)
	require.False(t, result.ThinkingCapabilities["broken"].Known)
}
