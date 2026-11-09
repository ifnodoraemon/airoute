package controlplane

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

type thinkingProbeRequest struct {
	ChannelID int64                 `json:"channel_id,omitempty"`
	BaseURL   string                `json:"base_url"`
	APIKey    string                `json:"api_key,omitempty"`
	Type      model.ProviderType    `json:"type"`
	Model     string                `json:"model"`
	Protocols []string              `json:"protocols,omitempty"`
	Thinking  *model.ThinkingConfig `json:"thinking,omitempty"`
}

// ProbeChannelThinking returns a reviewable result; saving channel facts remains
// the existing channel update operation. Stored credentials never leave here.
func (h *AdminHandler) ProbeChannelThinking(c *gin.Context) {
	var request thinkingProbeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "思考能力探测请求格式无效"})
		return
	}
	request.Model, request.BaseURL = strings.TrimSpace(request.Model), strings.TrimSpace(request.BaseURL)
	if request.Model == "" || strings.Contains(request.Model, "*") || validateProbeURL(request.BaseURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的服务地址及实际模型名称"})
		return
	}
	if request.ChannelID != 0 && request.APIKey == "" {
		var ok bool
		request.APIKey, ok = h.storedProbeCredential(c, request.ChannelID, request.BaseURL)
		if !ok {
			return
		}
	}
	channel := &model.ChannelConfig{Type: request.Type, BaseURL: request.BaseURL, APIKey: request.APIKey, Protocols: request.Protocols}
	if request.Thinking != nil {
		channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{request.Model: {Thinking: request.Thinking}}
		if err := model.ValidateModelCapabilities(channel.ModelCapabilities); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "思考档位配置无效，请检查档位名称及数量"})
			return
		}
	}
	result := h.prober.ProbeThinking(c.Request.Context(), channel, request.Model)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func (h *AdminHandler) storedProbeCredential(c *gin.Context, id int64, baseURL string) (string, bool) {
	channels, err := h.repo.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取渠道配置失败"})
		return "", false
	}
	for _, channel := range channels {
		if channel.ID != id {
			continue
		}
		// An edited destination must not receive a stored credential. The operator
		// must provide a new key for it explicitly, matching the save workflow.
		if strings.TrimRight(channel.BaseURL, "/") != strings.TrimRight(baseURL, "/") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "服务地址已变更，请填写该地址的 API Key 后再探测"})
			return "", false
		}
		return channel.APIKey, true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定渠道"})
	return "", false
}
