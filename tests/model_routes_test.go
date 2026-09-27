package tests

import (
	"testing"

	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestModelRoutesAggregationAndWeightPercent(t *testing.T) {
	channels := []model.ChannelConfig{
		{
			ID:       1,
			Name:     "upstream-primary",
			Type:     model.ProviderOpenAI,
			BaseURL:  "http://upstream-1:8080/v1",
			Models:   []string{"deepseek-chat", "gpt-4o"},
			Priority: 1,
			Weight:   80,
			Status:   "active",
		},
		{
			ID:       2,
			Name:     "upstream-secondary",
			Type:     model.ProviderOpenAI,
			BaseURL:  "http://upstream-2:8080/v1",
			Models:   []string{"deepseek-chat"},
			Priority: 1,
			Weight:   20,
			Status:   "active",
		},
		{
			ID:           3,
			Name:         "claude-backup",
			Type:         model.ProviderAnthropic,
			BaseURL:      "https://api.anthropic.com",
			Models:       []string{"deepseek-chat"},
			ModelMapping: map[string]string{"deepseek-chat": "claude-3-5-sonnet-20241022"},
			Priority:     2,
			Weight:       10,
			Status:       "active",
		},
	}

	d := router.NewDispatcher(channels)
	d.SetModelFallbacks(map[string]string{
		"deepseek-chat": "gpt-4o",
	})

	routes := d.GetModelRoutes()
	assert.NotEmpty(t, routes)

	var chatRoute *router.ModelRouteDetail
	for _, r := range routes {
		if r.Model == "deepseek-chat" {
			chatRoute = r
			break
		}
	}

	assert.NotNil(t, chatRoute, "deepseek-chat route must be found")
	assert.Equal(t, "chat", chatRoute.Modality)
	assert.Equal(t, "gpt-4o", chatRoute.FallbackModel)
	assert.Equal(t, 2, chatRoute.PrimaryProvidersCount)
	assert.Equal(t, 1, chatRoute.FallbackProvidersCount)
	assert.Equal(t, 100, chatRoute.TotalPrimaryWeight)
	assert.True(t, chatRoute.HasCrossProtocol, "should detect Anthropic provider as cross-protocol")
	assert.True(t, chatRoute.HasFallbackTier, "should detect priority 2 as fallback tier")

	// Verify provider weight percentages
	assert.Equal(t, 80.0, chatRoute.Providers[0].WeightPercent)
	assert.Equal(t, 20.0, chatRoute.Providers[1].WeightPercent)

	// Verify mapped model on backup
	assert.Equal(t, "claude-3-5-sonnet-20241022", chatRoute.Providers[2].MappedModel)
	assert.True(t, chatRoute.Providers[2].ConversionNeeded)
	assert.Equal(t, 2, chatRoute.Providers[2].Priority)
}

func TestCrossModelFallback(t *testing.T) {
	// Create mock channels where target-a is completely down (or not configured)
	// and target-b is healthy
	channels := []model.ChannelConfig{
		{
			ID:       1,
			Name:     "backup-provider",
			Type:     model.ProviderOpenAI,
			BaseURL:  "http://mock-upstream:8080/v1",
			Models:   []string{"backup-model"},
			Priority: 1,
			Weight:   10,
			Status:   "active",
		},
	}

	d := router.NewDispatcher(channels)
	d.SetModelFallbacks(map[string]string{
		"primary-model": "backup-model",
	})

	// When primary-model has no channels, fallback to backup-model channels
	matched := d.GetChannelsForModel("primary-model")
	assert.Empty(t, matched)

	// In dispatch, verify cross-model fallback is detected
	fb := d.GetModelFallback("primary-model")
	assert.Equal(t, "backup-model", fb)

	fbMatched := d.GetChannelsForModel(fb)
	assert.Len(t, fbMatched, 1)
	assert.Equal(t, "backup-provider", fbMatched[0].Name)
}

func TestModelRouteInferModality(t *testing.T) {
	channels := []model.ChannelConfig{
		{
			Name:     "all-models",
			Type:     model.ProviderOpenAI,
			BaseURL:  "http://upstream:8080/v1",
			Models:   []string{"dall-e-3", "tts-1", "whisper-1", "text-embedding-3-small", "bge-reranker-large", "sora", "deepseek-chat"},
			Priority: 1,
			Weight:   10,
		},
	}
	d := router.NewDispatcher(channels)
	routes := d.GetModelRoutes()

	modalityMap := make(map[string]string)
	for _, r := range routes {
		modalityMap[r.Model] = r.Modality
	}

	assert.Equal(t, "images", modalityMap["dall-e-3"])
	assert.Equal(t, "audio_speech", modalityMap["tts-1"])
	assert.Equal(t, "audio_transcription", modalityMap["whisper-1"])
	assert.Equal(t, "embeddings", modalityMap["text-embedding-3-small"])
	assert.Equal(t, "rerank", modalityMap["bge-reranker-large"])
	assert.Equal(t, "videos", modalityMap["sora"])
	assert.Equal(t, "chat", modalityMap["deepseek-chat"])
}
