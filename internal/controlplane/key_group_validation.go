package controlplane

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Keys select from the same configured groups the pricing overview exposes.
// Assigning a key must never implicitly create a pricing group.
func (h *AdminHandler) validateKeyPricingGroup(c *gin.Context, group string) bool {
	group = normalizeKeyGroup(group)
	if group == "default" {
		return true
	}
	overview, err := h.pricing.Overview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取计费分组失败，请稍后重试"})
		return false
	}
	if _, exists := overview.GroupMultipliers[group]; exists {
		return true
	}
	for _, rate := range overview.Rates {
		if normalizeKeyGroup(rate.GroupName) == group {
			return true
		}
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("计费分组 [%s] 不存在，请选择已有分组；新增分组请前往定价管理", group)})
	return false
}
