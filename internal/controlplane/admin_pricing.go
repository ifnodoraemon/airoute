package controlplane

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// These handlers are HTTP adapters over PricingService: they bind transport
// input, delegate the rules, and map the result onto a status code.

// writePricingError maps a PricingService error onto an HTTP response. fallback
// is the stage prefix the console shows for a storage failure; an empty fallback
// passes the underlying message through unchanged.
func writePricingError(c *gin.Context, err error, fallback string) {
	var verr *ValidationError
	if errors.As(err, &verr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": verr.Message})
		return
	}
	// The write committed but the in-memory state did not follow. Answering "failed"
	// would send the operator to re-enter a rate that is already saved; answering
	// plain success would hide that traffic is still priced by the previous rate.
	if writeHotReloadWarning(c, err, nil) {
		return
	}
	if fallback == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": fallback + ": " + err.Error()})
}

// GetPricingRates returns all model pricing rates, group multipliers, and cache savings benchmark.
func (h *AdminHandler) GetPricingRates(c *gin.Context) {
	overview, err := h.pricing.Overview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型计费单价列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":              0,
		"data":              overview.Rates,
		"group_multipliers": overview.GroupMultipliers,
		"server_time":       overview.ServerTime,
		"server_weekday":    overview.ServerWeekday,
		"is_weekend":        overview.IsWeekend,
	})
}

// SavePricingRateRequest is the wire shape of a pricing save.
//
// It is deliberately separate from storage.ModelPriceRecord. Binding a database
// row straight onto a request body makes every storage change an API change, and
// it also let a client post id / created_at / updated_at: columns the repository
// never reads from this path, but which the API appeared to accept.
//
// The accepted set is exactly the columns SaveModelPrice writes. That equality is
// enforced by TestSavePricingRateRequestAcceptsExactlyTheWritableColumns, so
// adding a column cannot silently leave the API behind.
type SavePricingRateRequest struct {
	Model           string  `json:"model"`
	GroupName       string  `json:"group_name"`
	PromptPrice     float64 `json:"prompt_price"`
	CompletionPrice float64 `json:"completion_price"`
	CacheReadPrice  float64 `json:"cache_read_price"`
	FixedPrice      float64 `json:"fixed_price"`
	Currency        string  `json:"currency"`
	OffPeakEnabled  bool    `json:"off_peak_enabled"`
	OffPeakMode     string  `json:"off_peak_mode"`
	OffPeakSlots    string  `json:"off_peak_slots"`
	WeekendAllDay   bool    `json:"weekend_all_day"`
	OffPeakStart    string  `json:"off_peak_start"`
	OffPeakEnd      string  `json:"off_peak_end"`
	OffPeakDiscount float64 `json:"off_peak_discount"`
}

// toRecord converts the request into the row the repository persists.
func (r SavePricingRateRequest) toRecord() *storage.ModelPriceRecord {
	return &storage.ModelPriceRecord{
		Model:           r.Model,
		GroupName:       r.GroupName,
		PromptPrice:     r.PromptPrice,
		CompletionPrice: r.CompletionPrice,
		CacheReadPrice:  r.CacheReadPrice,
		FixedPrice:      r.FixedPrice,
		Currency:        r.Currency,
		OffPeakEnabled:  r.OffPeakEnabled,
		OffPeakMode:     r.OffPeakMode,
		OffPeakSlots:    r.OffPeakSlots,
		WeekendAllDay:   r.WeekendAllDay,
		OffPeakStart:    r.OffPeakStart,
		OffPeakEnd:      r.OffPeakEnd,
		OffPeakDiscount: r.OffPeakDiscount,
	}
}

// SavePricingRate creates or updates pricing for a model.
func (h *AdminHandler) SavePricingRate(c *gin.Context) {
	var req SavePricingRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.pricing.SaveRate(c.Request.Context(), req.toRecord()); err != nil {
		writePricingError(c, err, "保存模型定价失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型计费单价配置已保存并实时生效"})
}

// DeletePricingRate removes pricing for a model (optionally within a group).
func (h *AdminHandler) DeletePricingRate(c *gin.Context) {
	modelName := c.Param("model")
	if modelName == "" {
		modelName = c.Query("model")
	}
	if err := h.pricing.DeleteRate(c.Request.Context(), modelName, c.Query("group")); err != nil {
		writePricingError(c, err, "删除模型定价失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除该模型计费规则"})
}

// BatchDeletePricingRates deletes pricing for multiple models.
func (h *AdminHandler) BatchDeletePricingRates(c *gin.Context) {
	var req struct {
		Models []string                `json:"models"`
		Items  []storage.ModelPriceKey `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}

	n, err := h.pricing.BatchDeleteRates(c.Request.Context(), req.Models, req.Items)
	if err != nil {
		writePricingError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个模型计费费率", n)})
}

// GetPricingGroups returns all configured pricing groups and their rate multipliers.
func (h *AdminHandler) GetPricingGroups(c *gin.Context) {
	multipliers := map[string]float64{"default": 1.0, "vip": 0.8, "enterprise": 1.5}
	if billing.GlobalEngine != nil {
		multipliers = billing.GlobalEngine.GetGroupMultipliers()
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": multipliers,
	})
}

// SavePricingGroups updates pricing group multipliers.
func (h *AdminHandler) SavePricingGroups(c *gin.Context) {
	var req map[string]float64
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误: " + err.Error()})
		return
	}
	if billing.GlobalEngine != nil {
		if err := billing.GlobalEngine.SetGroupMultipliers(req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存分组倍率失败: " + err.Error()})
			return
		}
	}
	if h.sync != nil {
		_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "pricing_groups_updated")
	}
	res := map[string]float64{"default": 1.0}
	if billing.GlobalEngine != nil {
		res = billing.GlobalEngine.GetGroupMultipliers()
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "分组计费倍率已保存并实时生效",
		"data":    res,
	})
}
