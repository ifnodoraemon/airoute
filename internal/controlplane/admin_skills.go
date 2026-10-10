package controlplane

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
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
		req.ID = "skill-" + strconv.FormatInt(time.Now().Unix(), 10)
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

	if err := h.repo.SaveSkill(record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "保存技能失败: " + err.Error()})
		return
	}
	_ = h.getStorage().Delete(c.Request.Context(), storage.FormatSkillKey(record.ID))
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "技能已保存", "data": record})
}

// DeleteSkill deletes an Agent Skill.
func (h *AdminHandler) DeleteSkill(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "技能 ID 不能为空"})
		return
	}
	if err := h.repo.DeleteSkill(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除技能失败: " + err.Error()})
		return
	}
	_ = h.getStorage().Delete(c.Request.Context(), storage.FormatSkillKey(id))
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "技能已删除"})
}

// DownloadSkillZip streams a standard .zip archive containing the skill bundle directory:
// <skill-id>/
// ├── SKILL.md
// ├── scripts/
// │   └── run.sh
// └── references/
//     └── metadata.json
// Supports caching to Local/RustFS/S3 and presigned URL redirection.
func (h *AdminHandler) DownloadSkillZip(c *gin.Context) {
	id := c.Param("id")
	skill, err := h.repo.GetSkill(id)
	if err != nil || skill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定的技能"})
		return
	}

	stor := h.getStorage()
	key := storage.FormatSkillKey(skill.ID)

	// 1. Ensure artifact is cached/stored in storage backend
	exists, _ := stor.Exists(c.Request.Context(), key)
	if !exists {
		// Pack dynamically in memory
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)

		// Root SKILL.md
		if wSkill, err := zw.Create(fmt.Sprintf("%s/SKILL.md", skill.ID)); err == nil {
			_, _ = wSkill.Write([]byte(skill.Manifest))
		}

		// scripts/run.sh helper
		if wScript, err := zw.Create(fmt.Sprintf("%s/scripts/run.sh", skill.ID)); err == nil {
			runScript := fmt.Sprintf("#!/usr/bin/env bash\n# Executable helper script for %s\n# agentskills.io open standard\necho \"[Skill %s] Executing task with tools: %s\"\n", skill.ID, skill.ID, strings.Join(skill.Tools, ", "))
			_, _ = wScript.Write([]byte(runScript))
		}

		// references/metadata.json
		if wRef, err := zw.Create(fmt.Sprintf("%s/references/metadata.json", skill.ID)); err == nil {
			metaBytes, _ := json.MarshalIndent(skill, "", "  ")
			_, _ = wRef.Write(metaBytes)
		}

		_ = zw.Close()
		zipBytes := buf.Bytes()

		putCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = stor.Put(putCtx, key, bytes.NewReader(zipBytes), int64(len(zipBytes)), "application/zip")
		cancel()
	}

	// 2. If an explicit public CDN / external domain prefix is configured, redirect to it.
	// Otherwise, keep the single public gateway entrypoint to avoid leaking internal hostnames (e.g. storage:9000).
	cfg := config.GetGlobalConfig().GetStorageConfig()
	if cfg.Driver == "s3" && cfg.S3.PublicURLPrefix != "" && !strings.Contains(cfg.S3.PublicURLPrefix, "storage:") {
		if presignedURL, err := stor.GetDownloadURL(c.Request.Context(), key, 30*time.Minute); err == nil && presignedURL != "" {
			c.Redirect(http.StatusFound, presignedURL)
			return
		}
	}

	// 3. Authoritative Gateway Single-Entrypoint Stream with HTTP Caching & Zero-Leakage
	etag := fmt.Sprintf("\"skill-%s-%s\"", skill.ID, skill.Version)
	if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", skill.ID))
	c.Header("Cache-Control", "public, max-age=86400, stale-while-revalidate=3600")
	c.Header("ETag", etag)

	rc, size, err := stor.Get(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取工件流失败: " + err.Error()})
		return
	}
	defer rc.Close()

	if size > 0 {
		c.Header("Content-Length", strconv.FormatInt(size, 10))
	}
	_, _ = io.Copy(c.Writer, rc)
}
