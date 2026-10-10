package controlplane

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// ProbeChannel handles automated downstream service detection and discovery.
func (h *AdminHandler) ProbeChannel(c *gin.Context) {
	var req ProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	telemetry.Logger.Info("handling downstream channel probe request",
		"base_url", req.BaseURL,
		"has_api_key", req.APIKey != "",
		"type", req.Type,
		"client_ip", c.ClientIP(),
	)

	result, err := h.prober.Probe(c.Request.Context(), &req)
	if err != nil {
		telemetry.Logger.Warn("downstream channel probe returned error",
			"base_url", req.BaseURL,
			"error", err.Error(),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	telemetry.Logger.Info("downstream channel probe completed",
		"base_url", req.BaseURL,
		"detected_type", result.Type,
		"suggested_name", result.SuggestedName,
		"suggested_base_url", result.SuggestedBaseURL,
		"models_count", len(result.Models),
		"latency_ms", result.LatencyMs,
		"message", result.Message,
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// TestChannel tests the live connectivity of a channel.
func (h *AdminHandler) TestChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return
	}

	channels, err := h.repo.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取渠道列表失败: " + err.Error()})
		return
	}

	var target *storage.ChannelRecord
	for _, ch := range channels {
		if ch.ID == id {
			target = ch
			break
		}
	}

	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定渠道"})
		return
	}

	testModel := "gpt-3.5-turbo"
	if len(target.Models) > 0 {
		testModel = target.Models[0]
	}

	chCfg := model.ChannelConfig{
		Name:           target.Name,
		Type:           target.Type,
		BaseURL:        target.BaseURL,
		APIKey:         target.APIKey,
		Models:         target.Models,
		ModelMapping:   target.ModelMapping,
		TimeoutSeconds: 15,
	}

	var prov provider.Provider
	switch target.Type {
	case model.ProviderAnthropic:
		prov = provider.NewAnthropicProvider(nil)
	case model.ProviderGemini:
		prov = provider.NewGeminiProvider(nil)
	default:
		prov = provider.NewOpenAIProvider(nil)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	testReq := &model.ChatCompletionRequest{
		Model: testModel,
		Messages: []model.ChatMessage{
			{Role: "user", Content: "ping"},
		},
	}

	start := time.Now()
	resp, err := prov.ChatComplete(ctx, testReq, &chCfg)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":       1,
			"success":    false,
			"latency_ms": latencyMs,
			"error":      err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"success":    true,
		"latency_ms": latencyMs,
		"response":   resp.Choices[0].Message.GetContentString(),
	})
}
