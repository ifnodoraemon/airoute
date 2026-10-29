package controlplane

import (
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"net/http"
	"strings"
)

// SaveMCPServerRequest payload for adding or updating an MCP Server.
type SaveMCPServerRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Transport   string   `json:"transport"`
	Endpoint    string   `json:"endpoint"`
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	Tools       []string `json:"tools"`
	Prompts     []string `json:"prompts"`
	Resources   []string `json:"resources"`
	EnvVars     string   `json:"env_vars"`
	Enabled     bool     `json:"enabled"`
}

// SaveMCPServer creates or updates a custom MCP Server.
func (h *AdminHandler) SaveMCPServer(c *gin.Context) {
	var req SaveMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请求参数不合法: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Endpoint) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "MCP 服务名称和 Endpoint 不能为空"})
		return
	}
	if req.ID == "" {
		req.ID = "custom-" + storage.NewBillingID()
	}
	if err := storage.ValidateEcosystemID(req.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}

	if req.Category == "" {
		req.Category = "dev"
	}
	if req.Transport == "" {
		req.Transport = "sse"
	}
	if req.Transport != "stdio" && req.Transport != "sse" && req.Transport != "http" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "transport 仅支持 stdio、sse 或 http"})
		return
	}
	if req.Transport == "stdio" {
		if _, err := model.ParseMCPCommand(req.Endpoint); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
			return
		}
	}
	if _, err := model.ParseMCPEnv(req.EnvVars); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}
	if len(req.Tools) == 0 {
		req.Tools = []string{req.ID + "_tool"}
	}

	record := &storage.MCPServerRecord{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Transport:   req.Transport,
		Endpoint:    req.Endpoint,
		Status:      "online",
		Author:      req.Author,
		Version:     req.Version,
		Tools:       req.Tools,
		Prompts:     req.Prompts,
		Resources:   req.Resources,
		EnvVars:     req.EnvVars,
		Enabled:     req.Enabled,
	}

	if err := h.repo.SaveMCPServer(record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "保存 MCP 服务器失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务保存成功", "data": record})
}

// ToggleMCPServer enables or disables an MCP Server.
func (h *AdminHandler) ToggleMCPServer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "MCP 服务 ID 不能为空"})
		return
	}

	var req ToggleSkillRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.Enabled != nil {
		if err := h.repo.SetMCPServerEnabled(id, *req.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新状态失败: " + err.Error()})
			return
		}
	} else {
		srv, err := h.repo.GetMCPServer(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "未找到该 MCP 服务"})
			return
		}
		if err := h.repo.SetMCPServerEnabled(id, !srv.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "切换状态失败: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务状态更新成功"})
}

// DeleteMCPServer deletes an MCP Server.
func (h *AdminHandler) DeleteMCPServer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "MCP 服务 ID 不能为空"})
		return
	}
	if err := h.repo.DeleteMCPServer(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除 MCP 服务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务已删除"})
}
