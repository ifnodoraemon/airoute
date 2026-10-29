package controlplane

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"net/http"
	"strings"
)

// ListSkills returns all Agent Skills with their current enabled states.
func (h *AdminHandler) ListSkills(c *gin.Context) {
	skills, err := h.repo.ListSkills()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "获取技能列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": skills})
}

// ToggleSkillRequest defines toggle payload.
type ToggleSkillRequest struct {
	Enabled *bool `json:"enabled"`
}

// ToggleSkill enables or disables an Agent Skill on-demand.
func (h *AdminHandler) ToggleSkill(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "技能 ID 不能为空"})
		return
	}

	// The enabled flag is part of references/metadata.json, so a toggle changes the
	// bundle content hash and therefore its object key. Capture the previous row so
	// the unreachable artifact can be dropped.
	previous, _ := h.repo.GetSkill(id)

	var req ToggleSkillRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.Enabled != nil {
		if err := h.repo.SetSkillEnabled(id, *req.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新技能状态失败: " + err.Error()})
			return
		}
	} else {
		current := h.repo.IsSkillEnabled(id)
		if err := h.repo.SetSkillEnabled(id, !current); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "切换技能状态失败: " + err.Error()})
			return
		}
	}

	if previous != nil {
		if previousKey, keyErr := skillArtifactKey(previous); keyErr == nil {
			_ = h.getStorage().Delete(c.Request.Context(), previousKey)
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "技能状态更新成功", "enabled": h.repo.IsSkillEnabled(id)})
}

// SaveSkillRequest defines payload for saving an Agent Skill.
type SaveSkillRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tools       []string `json:"tools"`
	LoadingMode string   `json:"loading_mode"`
	Manifest    string   `json:"manifest"`
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	Enabled     bool     `json:"enabled"`
}

// SaveSkill creates or updates an Agent Skill in Skill Hub.
func (h *AdminHandler) SaveSkill(c *gin.Context) {
	var req SaveSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请求参数不合法: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Description) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "技能名称和描述不能为空"})
		return
	}
	if req.ID == "" {
		req.ID = "skill-" + storage.NewBillingID()
	}
	if err := storage.ValidateEcosystemID(req.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}

	if req.Category == "" {
		req.Category = "custom"
	}
	if req.LoadingMode == "" {
		req.LoadingMode = "lazy"
	}
	if len(req.Tools) == 0 {
		req.Tools = []string{req.ID + "_tool"}
	}
	if req.Manifest == "" {
		req.Manifest = fmt.Sprintf("---\nname: %s\ndescription: %s\ncategory: %s\n---\n\n# %s\n\n%s", req.ID, req.Description, req.Category, req.Name, req.Description)
	}

	record := &storage.SkillRecord{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Tools:       req.Tools,
		LoadingMode: req.LoadingMode,
		Manifest:    req.Manifest,
		Author:      req.Author,
		Version:     req.Version,
		Enabled:     req.Enabled,
	}

	// The bundle is content-addressed, so a content edit produces a new object key
	// even when the version string is unchanged. Drop the previous bundle and the
	// legacy id-only key so the storage backend does not accumulate artifacts that
	// no request can ever reach again.
	previous, _ := h.repo.GetSkill(record.ID)
	if err := h.repo.SaveSkill(record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "保存技能失败: " + err.Error()})
		return
	}
	stor := h.getStorage()
	_ = stor.Delete(c.Request.Context(), storage.FormatSkillKey(record.ID))
	if previous != nil {
		if previousKey, keyErr := skillArtifactKey(previous); keyErr == nil {
			_ = stor.Delete(c.Request.Context(), previousKey)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "技能已保存", "data": record})
}

// DeleteSkill deletes an Agent Skill.
func (h *AdminHandler) DeleteSkill(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "技能 ID 不能为空"})
		return
	}
	previous, _ := h.repo.GetSkill(id)
	if err := h.repo.DeleteSkill(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除技能失败: " + err.Error()})
		return
	}
	stor := h.getStorage()
	_ = stor.Delete(c.Request.Context(), storage.FormatSkillKey(id))
	if previous != nil {
		if previousKey, keyErr := skillArtifactKey(previous); keyErr == nil {
			_ = stor.Delete(c.Request.Context(), previousKey)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "技能已删除"})
}
