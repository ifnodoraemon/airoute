package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SystemAnnouncement represents a platform-wide broadcast announcement.
type SystemAnnouncement struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Level       string    `json:"level"` // "info" | "warning" | "critical"
	Enabled     bool      `json:"enabled"`
	PublishedAt time.Time `json:"published_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const systemAnnouncementSettingKey = "system_announcement"

// GetAnnouncement returns the active announcement for all users (public / protected).
// If no announcement is configured or if it is disabled, returns code: 0, data: null.
func (h *AdminHandler) GetAnnouncement(c *gin.Context) {
	val := h.repo.GetSetting(systemAnnouncementSettingKey, "")
	if val == "" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
		return
	}

	var ann SystemAnnouncement
	if err := json.Unmarshal([]byte(val), &ann); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
		return
	}

	if !ann.Enabled {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": ann})
}

// GetAdminAnnouncement returns the announcement configuration for administrators,
// regardless of whether it is enabled or disabled.
func (h *AdminHandler) GetAdminAnnouncement(c *gin.Context) {
	val := h.repo.GetSetting(systemAnnouncementSettingKey, "")
	if val == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": SystemAnnouncement{
				ID:      "",
				Title:   "",
				Content: "",
				Level:   "info",
				Enabled: false,
			},
		})
		return
	}

	var ann SystemAnnouncement
	if err := json.Unmarshal([]byte(val), &ann); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": SystemAnnouncement{
				ID:      "",
				Title:   "",
				Content: "",
				Level:   "info",
				Enabled: false,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": ann})
}

// SaveAnnouncement saves and publishes an announcement (super-admin only).
func (h *AdminHandler) SaveAnnouncement(c *gin.Context) {
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Level   string `json:"level"`
		Enabled bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的公告请求参数"})
		return
	}

	level := req.Level
	if level != "warning" && level != "critical" {
		level = "info"
	}

	now := time.Now().UTC()
	annID := fmt.Sprintf("ann_%d", now.Unix())

	ann := SystemAnnouncement{
		ID:          annID,
		Title:       req.Title,
		Content:     req.Content,
		Level:       level,
		Enabled:     req.Enabled,
		PublishedAt: now,
		UpdatedAt:   now,
	}

	bytes, err := json.Marshal(ann)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化公告失败"})
		return
	}

	if err := h.repo.SetSetting(systemAnnouncementSettingKey, string(bytes)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存公告失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "系统公告配置已保存",
		"data":    ann,
	})
}
