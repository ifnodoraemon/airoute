package controlplane

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// skillArtifactHashLength is how much of the SHA-256 content digest is embedded in
// the artifact key and the ETag. 16 hex characters (64 bits) is plenty to separate
// revisions of one skill while keeping object keys readable.
const skillArtifactHashLength = 16

// skillZipBytes renders the downloadable bundle for a skill.
//
// This is the single definition of the bundle content: the download path serves
// exactly these bytes and the content hash is taken over them, so the artifact key
// can never name content it does not contain.
func skillZipBytes(skill *storage.SkillRecord) ([]byte, error) {
	if skill == nil {
		return nil, fmt.Errorf("nil skill")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Root SKILL.md
	wSkill, err := zw.Create(fmt.Sprintf("%s/SKILL.md", skill.ID))
	if err != nil {
		return nil, err
	}
	if _, err := wSkill.Write([]byte(skill.Manifest)); err != nil {
		return nil, err
	}

	// scripts/run.sh helper
	wScript, err := zw.Create(fmt.Sprintf("%s/scripts/run.sh", skill.ID))
	if err != nil {
		return nil, err
	}
	runScript := fmt.Sprintf("#!/usr/bin/env bash\n# Executable helper script for %s\n# agentskills.io open standard\necho \"[Skill %s] Executing task with tools: %s\"\n", skill.ID, skill.ID, strings.Join(skill.Tools, ", "))
	if _, err := wScript.Write([]byte(runScript)); err != nil {
		return nil, err
	}

	// references/metadata.json
	wRef, err := zw.Create(fmt.Sprintf("%s/references/metadata.json", skill.ID))
	if err != nil {
		return nil, err
	}
	metaBytes, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err := wRef.Write(metaBytes); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// skillArtifactFingerprint hashes the bundle content.
//
// The identity of a downloadable artifact used to be (id, version), so editing a
// manifest without bumping the version served the previously cached zip forever.
// Hashing the bytes makes the identity the content itself.
func skillArtifactFingerprint(skill *storage.SkillRecord) (string, error) {
	zipBytes, err := skillZipBytes(skill)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(zipBytes)
	return hex.EncodeToString(sum[:])[:skillArtifactHashLength], nil
}

// formatSkillArtifactKey renders the content-addressed object key of a bundle.
func formatSkillArtifactKey(skillID, fingerprint string) string {
	return fmt.Sprintf("skills/%s/%s-%s.zip", skillID, skillID, fingerprint)
}

// skillArtifactKey returns the content-addressed storage key of a skill bundle.
func skillArtifactKey(skill *storage.SkillRecord) (string, error) {
	fingerprint, err := skillArtifactFingerprint(skill)
	if err != nil {
		return "", err
	}
	return formatSkillArtifactKey(skill.ID, fingerprint), nil
}

// etagMatches reports whether an If-None-Match header lists etag.
//
// The header is a comma-separated list and browsers may send the weak form
// (W/"..."), so an exact string compare — what the handler used to do — silently
// misses some revalidations and re-sends the whole zip instead of a 304.
func etagMatches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" || etag == "" {
		return false
	}
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
		if strings.HasPrefix(candidate, "W/") && strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

// DownloadSkillZip streams a standard .zip archive containing the skill bundle directory:
// <skill-id>/
// ├── SKILL.md
// ├── scripts/
// │   └── run.sh
// └── references/
//
//	└── metadata.json
//
// Supports caching to Local/RustFS/S3 and presigned URL redirection.
func (h *AdminHandler) DownloadSkillZip(c *gin.Context) {
	id := c.Param("id")
	skill, err := h.repo.GetSkill(id)
	if err != nil || skill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定的技能"})
		return
	}

	zipBytes, err := skillZipBytes(skill)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "打包技能工件失败: " + err.Error()})
		return
	}
	sum := sha256.Sum256(zipBytes)
	fingerprint := hex.EncodeToString(sum[:])[:skillArtifactHashLength]

	stor := h.getStorage()
	key := formatSkillArtifactKey(skill.ID, fingerprint)

	// 1. Ensure artifact is cached/stored in storage backend. The key embeds the
	// content hash, so an edited manifest writes a new object instead of reusing
	// the zip cached for the previous content.
	exists, _ := stor.Exists(c.Request.Context(), key)
	if !exists {
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

	// 3. Authoritative Gateway Single-Entrypoint Stream with HTTP Caching & Zero-Leakage.
	//
	// The ETag carries the content hash, not just id+version: content can change
	// without a version bump, and with the old ETag the browser (and the day-long
	// max-age) kept serving the stale zip after an edit. Responses now revalidate on
	// every use, and a 304 is only sent for genuinely identical content.
	etag := fmt.Sprintf("\"skill-%s-%s-%s\"", skill.ID, skill.Version, fingerprint)
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", skill.ID))
	c.Header("Cache-Control", "public, max-age=0, must-revalidate")
	c.Header("ETag", etag)
	if etagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}

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
