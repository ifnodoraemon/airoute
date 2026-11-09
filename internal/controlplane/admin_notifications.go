package controlplane

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type SendNotificationRequest struct {
	RequestID         string `json:"request_id"`
	RecipientUsername string `json:"recipient_username"`
	Title             string `json:"title"`
	Content           string `json:"content"`
	Level             string `json:"level"`
}

func (h *AdminHandler) SendNotification(c *gin.Context) {
	if _, ok := RequireAdminClaims(c, "仅管理员可以发送站内通知"); !ok {
		return
	}
	var req SendNotificationRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, notificationBodyLimit)
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的通知请求参数"})
		return
	}
	if storage.ValidateEcosystemID(req.RequestID) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "发送请求标识无效"})
		return
	}
	req.RecipientUsername = strings.TrimSpace(req.RecipientUsername)
	if req.RecipientUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写收件账号"})
		return
	}
	if err := normalizeNotificationText(&req.Title, &req.Content, &req.Level, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rec, err := h.repo.SendNotification(c.Request.Context(), req.RequestID, req.RecipientUsername, req.Title, req.Content, req.Level)
	if errors.Is(err, storage.ErrNotificationRecipient) {
		c.JSON(http.StatusNotFound, gin.H{"error": "收件账号不存在"})
		return
	}
	if errors.Is(err, storage.ErrNotificationConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": "该发送请求已使用，请刷新后重试"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发送通知失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "站内通知已发送", "data": rec})
}
