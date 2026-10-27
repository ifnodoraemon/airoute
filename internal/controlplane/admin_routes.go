package controlplane

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UpdateModelRouteRequest specifies model-level distribution, mapping, and fallback settings.
type UpdateModelRouteRequest struct {
	Model string `json:"model" binding:"required"`
	// GroupName selects the pricing group being edited. Empty means "default".
	// Without it, editing a non-default group silently read and wrote the
	// default group's rates.
	GroupName       string                `json:"group_name,omitempty"`
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
//
// The handler is an HTTP adapter: it binds the request, delegates every rule to
// RouteService, and maps the stage of a failure onto the status code and message
// the console expects.
func (h *AdminHandler) UpdateModelRoute(c *gin.Context) {
	var req UpdateModelRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.routes.UpdateModelRoute(c.Request.Context(), &req); err != nil {
		writeRouteError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型多源路由及分发容灾配置已保存并完成热重载"})
}

// writeRouteError maps a RouteService error onto the response the console
// expects. Each stage keeps its own message: "nothing was saved" and "saved but
// the data plane did not pick it up" are different situations for an operator.
func writeRouteError(c *gin.Context, err error) {
	var verr *ValidationError
	if errors.As(err, &verr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": verr.Message})
		return
	}

	var serr *RouteStageError
	if errors.As(err, &serr) {
		switch serr.Stage {
		case RouteStageRead:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取现有定价失败，未保存任何配置: " + serr.Err.Error()})
			return
		case RouteStageCollect:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新渠道配置失败: " + serr.Err.Error()})
			return
		case RouteStagePersist:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存模型路由失败（已回滚，未改动任何配置）: " + serr.Err.Error()})
			return
		case RouteStageApply:
			writeHotReloadWarning(c, &HotReloadError{Stage: "模型路由", Err: serr.Err}, nil)
			return
		}
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
