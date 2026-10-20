package controlplane

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func (h *AdminHandler) ValidateRoutingChannel(c *gin.Context) {
	if _, ok := RequireAdminClaims(c, "仅管理员可验证模型渠道"); !ok {
		return
	}
	var req struct {
		Model     string `json:"model" binding:"required"`
		ChannelID int64  `json:"channel_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请指定模型和渠道"})
		return
	}
	if h.dispatcher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "调度器尚未就绪"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	result, err := h.dispatcher.ValidateModelChannel(ctx, req.Model, req.ChannelID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, router.ErrRoutingProbeTarget) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}
