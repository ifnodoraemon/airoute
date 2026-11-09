package controlplane

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type AnnouncementRequest struct {
	RequestID  string `json:"request_id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Level      string `json:"level"`
	Status     string `json:"status"`
	ShowBanner bool   `json:"show_banner"`
}

func (h *AdminHandler) GetAnnouncementBanner(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rec, err := h.repo.GetAnnouncementBanner(c.Request.Context())
	announcementResponse(c, rec, err)
}

func (h *AdminHandler) GetAnnouncement(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rec, err := h.repo.GetAnnouncement(c.Request.Context(), c.Param("id"))
	announcementResponse(c, rec, err)
}

func (h *AdminHandler) ListAnnouncements(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status := c.Query("status")
	if status != "" && status != "draft" && status != "published" {
		c.JSON(400, gin.H{"error": "公告状态无效"})
		return
	}
	page, err := notificationPageValue(c.DefaultQuery("page", "1"), 100000)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	size, err := notificationPageValue(c.DefaultQuery("page_size", "20"), 100)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	list, err := h.repo.ListAnnouncements(c.Request.Context(), status, page, size)
	announcementResponse(c, list, err)
}

func (h *AdminHandler) CreateAnnouncement(c *gin.Context) { h.writeAnnouncement(c, true) }
func (h *AdminHandler) UpdateAnnouncement(c *gin.Context) { h.writeAnnouncement(c, false) }

func (h *AdminHandler) writeAnnouncement(c *gin.Context, create bool) {
	if _, ok := RequireAdminClaims(c, "仅管理员可以管理全站公告"); !ok {
		return
	}
	var req AnnouncementRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, notificationBodyLimit)
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "无效的公告参数"})
		return
	}
	if req.Status != "draft" && req.Status != "published" {
		c.JSON(400, gin.H{"error": "请选择草稿或发布状态"})
		return
	}
	if create && storage.ValidateEcosystemID(req.RequestID) != nil {
		c.JSON(400, gin.H{"error": "发布请求标识无效"})
		return
	}
	if err := normalizeNotificationText(&req.Title, &req.Content, &req.Level, req.Status == "published"); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	draft := storage.AnnouncementRecord{Title: req.Title, Content: req.Content, Level: req.Level, Status: req.Status, ShowBanner: req.ShowBanner}
	var rec *storage.AnnouncementRecord
	var err error
	if create {
		rec, err = h.repo.CreateAnnouncement(c.Request.Context(), req.RequestID, draft)
	} else {
		rec, err = h.repo.UpdateAnnouncement(c.Request.Context(), c.Param("id"), draft)
	}
	announcementResponse(c, rec, err)
}

func (h *AdminHandler) DeleteAnnouncement(c *gin.Context) {
	if _, ok := RequireAdminClaims(c, "仅管理员可以删除公告"); !ok {
		return
	}
	err := h.repo.DeleteAnnouncement(c.Request.Context(), c.Param("id"))
	announcementResponse(c, nil, err)
}

func announcementResponse(c *gin.Context, data any, err error) {
	switch {
	case errors.Is(err, storage.ErrAnnouncementNotFound):
		c.JSON(404, gin.H{"error": "公告不存在"})
	case errors.Is(err, storage.ErrAnnouncementConflict):
		c.JSON(409, gin.H{"error": "该发布请求标识已使用"})
	case err != nil:
		c.JSON(500, gin.H{"error": "公告操作失败，请稍后重试"})
	default:
		c.JSON(200, gin.H{"code": 0, "data": data})
	}
}
