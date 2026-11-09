package controlplane

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type MarkAllNotificationsReadRequest struct {
	ThroughSequence int64  `json:"through_sequence"`
	Kind            string `json:"kind"`
}

func (h *AdminHandler) ListNotifications(c *gin.Context) {
	userID, ok := requireNotificationUser(c)
	if !ok {
		return
	}
	filter, err := notificationFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := h.repo.ListNotifications(c.Request.Context(), userID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取消息失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": page})
}

func (h *AdminHandler) GetNotificationUnreadCount(c *gin.Context) {
	userID, ok := requireNotificationUser(c)
	if !ok {
		return
	}
	counts, err := h.repo.NotificationUnreadCounts(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取未读消息数量失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": counts})
}

func (h *AdminHandler) MarkNotificationRead(c *gin.Context) {
	userID, ok := requireNotificationUser(c)
	if !ok {
		return
	}
	err := h.repo.MarkNotificationRead(c.Request.Context(), userID, c.Param("id"))
	if errors.Is(err, storage.ErrNotificationNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "消息不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "标记已读失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

func (h *AdminHandler) MarkAllNotificationsRead(c *gin.Context) {
	userID, ok := requireNotificationUser(c)
	if !ok {
		return
	}
	var req MarkAllNotificationsReadRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if c.ShouldBindJSON(&req) != nil || req.ThroughSequence <= 0 || (req.Kind != "announcement" && req.Kind != "message") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的消息快照与类型"})
		return
	}
	if err := h.repo.MarkAllNotificationsRead(c.Request.Context(), userID, req.ThroughSequence, req.Kind); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "标记全部已读失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}
