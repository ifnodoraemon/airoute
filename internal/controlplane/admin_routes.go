package controlplane

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// UpdateModelRouteRequest specifies model-level distribution, mapping, and fallback settings.
type UpdateModelRouteRequest struct {
	Model           string                `json:"model" binding:"required"`
	FallbackModel   string                `json:"fallback_model"`
	ProviderUpdates []ModelProviderUpdate `json:"provider_updates"`
	PromptPrice     *float64              `json:"prompt_price,omitempty"`
	CompletionPrice *float64              `json:"completion_price,omitempty"`
	CacheReadPrice  *float64              `json:"cache_read_price,omitempty"`
	FixedPrice      *float64              `json:"fixed_price,omitempty"`
	Currency        string                `json:"currency,omitempty"`
	OffPeakEnabled  *bool                 `json:"off_peak_enabled,omitempty"`
	OffPeakMode     string                `json:"off_peak_mode,omitempty"`
	OffPeakSlots    string                `json:"off_peak_slots,omitempty"`
	WeekendAllDay   *bool                 `json:"weekend_all_day,omitempty"`
	OffPeakStart    string                `json:"off_peak_start,omitempty"`
	OffPeakEnd      string                `json:"off_peak_end,omitempty"`
	OffPeakDiscount *float64              `json:"off_peak_discount,omitempty"`
}

// ModelProviderUpdate describes changes to a provider's weight, priority, or mapped model.
type ModelProviderUpdate struct {
	ChannelID   int64  `json:"channel_id"`
	Priority    int    `json:"priority"`
	Weight      int    `json:"weight"`
	MappedModel string `json:"mapped_model"`
}

// GetModelRoutes returns enriched model-centric routing topology, weights, and fallback rules.
func (h *AdminHandler) GetModelRoutes(c *gin.Context) {
	routes := h.dispatcher.GetModelRoutes()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": routes})
}

// UpdateModelRoute updates provider weights, priorities, mapped models, and cross-model fallback.
func (h *AdminHandler) UpdateModelRoute(c *gin.Context) {
	var req UpdateModelRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模型标识不能为空"})
		return
	}

	// 1. Update Fallback model
	if req.FallbackModel != "" && req.FallbackModel != req.Model {
		if err := h.repo.SetModelFallback(req.Model, req.FallbackModel, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存模型兜底失败: " + err.Error()})
			return
		}
	} else {
		_ = h.repo.DeleteModelFallback(req.Model)
	}

	// 2. Save model pricing if provided
	if err := h.saveRoutePricing(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. Update channels: bind desired channels, unbind removed channels
	if err := h.applyChannelUpdatesForModel(req.Model, req.ProviderUpdates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新渠道配置失败: " + err.Error()})
		return
	}

	// 4. Atomically sync DB state into Dispatcher and broadcast to Redis cluster
	if err := h.sync.ReloadAndBroadcast(c.Request.Context(), "model_route_updated"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据面热重载失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型多源路由及分发容灾配置已保存并完成热重载"})
}


func (h *AdminHandler) applyChannelUpdatesForModel(modelName string, providerUpdates []ModelProviderUpdate) error {
	desiredProviders := make(map[int64]ModelProviderUpdate, len(providerUpdates))
	for _, pu := range providerUpdates {
		desiredProviders[pu.ChannelID] = pu
	}

	allChannels, err := h.repo.ListChannels()
	if err != nil {
		return err
	}

	for _, ch := range allChannels {
		pu, isDesired := desiredProviders[ch.ID]
		changed := false

		containsModel := false
		for _, m := range ch.Models {
			if m == modelName {
				containsModel = true
				break
			}
		}

		if isDesired {
			if !containsModel {
				ch.Models = append(ch.Models, modelName)
				changed = true
			}
			if pu.Priority > 0 && ch.Priority != pu.Priority {
				ch.Priority = pu.Priority
				changed = true
			}
			if pu.Weight > 0 && ch.Weight != pu.Weight {
				ch.Weight = pu.Weight
				changed = true
			}
			if ch.ModelMapping == nil {
				ch.ModelMapping = make(map[string]string)
			}
			if pu.MappedModel != "" && pu.MappedModel != modelName {
				if ch.ModelMapping[modelName] != pu.MappedModel {
					ch.ModelMapping[modelName] = pu.MappedModel
					changed = true
				}
			} else {
				if _, exists := ch.ModelMapping[modelName]; exists {
					delete(ch.ModelMapping, modelName)
					changed = true
				}
			}
		} else {
			if containsModel {
				var remaining []string
				for _, m := range ch.Models {
					if m != modelName {
						remaining = append(remaining, m)
					}
				}
				ch.Models = remaining
				changed = true
			}
			if ch.ModelMapping != nil {
				if _, exists := ch.ModelMapping[modelName]; exists {
					delete(ch.ModelMapping, modelName)
					changed = true
				}
			}
		}

		if changed {
			if err := h.repo.UpdateChannel(ch); err != nil {
				return err
			}
		}
	}
	return nil
}

