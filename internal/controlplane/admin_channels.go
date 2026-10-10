package controlplane

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListChannels returns all configured channels.
func (h *AdminHandler) ListChannels(c *gin.Context) {
	channels, err := h.repo.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取渠道列表失败: " + err.Error()})
		return
	}
	if h.dispatcher != nil {
		for _, ch := range channels {
			ch.BreakerStatus = h.dispatcher.GetBreakerStatus(ch.Name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": channels})
}

// CreateChannel creates a new channel.
func (h *AdminHandler) CreateChannel(c *gin.Context) {
	var rec storage.ChannelRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if rec.Name == "" || rec.BaseURL == "" || rec.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道名称、类型与 base_url 为必填项"})
		return
	}

	if err := h.repo.CreateChannel(&rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建渠道失败: " + err.Error()})
		return
	}

	// Trigger hot reload into Data Plane memory and broadcast to cluster
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channel_created")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rec, "message": "渠道已创建并成功同步至数据面内存"})
}

// UpdateChannel updates an existing channel.
func (h *AdminHandler) UpdateChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return
	}

	var rec storage.ChannelRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rec.ID = id

	if err := h.repo.UpdateChannel(&rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新渠道失败: " + err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channel_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rec, "message": "渠道配置已成功更新"})
}

// DeleteChannel removes a channel.
func (h *AdminHandler) DeleteChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return
	}

	if err := h.repo.DeleteChannel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除渠道失败: " + err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channel_deleted")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "渠道已成功删除"})
}

// BatchDeleteChannels deletes multiple channels.
func (h *AdminHandler) BatchDeleteChannels(c *gin.Context) {
	ids, ok := h.bindBatchIDs(c, "请提供有效渠道 ID 列表")
	if !ok {
		return
	}
	n, err := h.repo.BatchDeleteChannels(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channels_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个服务商渠道", n), "deleted_count": n})
}

// BatchStatusChannels toggles status for multiple channels.
func (h *AdminHandler) BatchStatusChannels(c *gin.Context) {
	req, ok := h.bindBatchStatus(c)
	if !ok {
		return
	}
	n, err := h.repo.BatchUpdateChannelStatus(req.IDs, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channels_batch_status_updated")
	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个服务商渠道", action, n)})
}
