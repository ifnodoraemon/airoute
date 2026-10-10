package controlplane

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// GetPricingRates returns all model pricing rates and cache savings benchmark.
func (h *AdminHandler) GetPricingRates(c *gin.Context) {
	rates, err := h.repo.ListModelPrices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型计费单价列表失败: " + err.Error()})
		return
	}

	type enrichedPrice struct {
		*storage.ModelPriceRecord
		IsCurrentOffPeak  bool    `json:"is_current_off_peak"`
		EffectiveDiscount float64 `json:"effective_discount"`
	}

	now := time.Now()
	loc, errLoc := time.LoadLocation("Asia/Shanghai")
	if errLoc != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	nowCST := now.In(loc)

	enriched := make([]enrichedPrice, 0, len(rates))
	for _, r := range rates {
		isOff, disc := false, 1.0
		if billing.GlobalEngine != nil {
			isOff, disc = billing.GlobalEngine.IsOffPeak(r, now)
		}
		enriched = append(enriched, enrichedPrice{
			ModelPriceRecord:  r,
			IsCurrentOffPeak:  isOff,
			EffectiveDiscount: disc,
		})
	}

	weekday := nowCST.Weekday()
	isWeekend := (weekday == time.Saturday || weekday == time.Sunday)

	c.JSON(http.StatusOK, gin.H{
		"code":           0,
		"data":           enriched,
		"server_time":    nowCST.Format("15:04:05"),
		"server_weekday": weekday.String(),
		"is_weekend":     isWeekend,
	})
}

// SavePricingRate creates or updates pricing for a model.
func (h *AdminHandler) SavePricingRate(c *gin.Context) {
	var req storage.ModelPriceRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模型标识为必填项"})
		return
	}
	if req.OffPeakEnabled && req.OffPeakSlots != "" {
		if err := billing.ValidateSlotsOverlap(req.OffPeakSlots); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.repo.SaveModelPrice(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存模型定价失败: " + err.Error()})
		return
	}

	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "model_price_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型计费单价配置已保存并实时生效"})
}

// DeletePricingRate removes pricing for a model (optionally within a group).
func (h *AdminHandler) DeletePricingRate(c *gin.Context) {
	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模型标识为必填项"})
		return
	}
	groupName := strings.TrimSpace(c.Query("group"))
	var err error
	if groupName != "" {
		err = h.repo.DeleteModelPriceWithGroup(modelName, groupName)
	} else {
		err = h.repo.DeleteModelPrice(modelName)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除模型定价失败: " + err.Error()})
		return
	}

	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "model_price_deleted")

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
	var n int64
	var err error
	if len(req.Items) > 0 {
		n, err = h.repo.BatchDeleteModelPriceKeys(req.Items)
	} else if len(req.Models) > 0 {
		n, err = h.repo.BatchDeleteModelPrices(req.Models)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供待删除定价模型列表"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "pricing_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个模型计费费率", n)})
}
