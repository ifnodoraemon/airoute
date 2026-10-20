package controlplane

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DeleteModelRoute removes a route policy and suppresses automatic recreation.
// The channel's supported models, aliases, credentials and defaults are retained.
func (h *AdminHandler) DeleteModelRoute(c *gin.Context) {
	name := c.Param("model")
	if name == "" {
		name = c.Query("model")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模型标识为必填项"})
		return
	}
	h.deleteModelRoutes(c, []string{name})
}

func (h *AdminHandler) BatchDeleteModelRoutes(c *gin.Context) {
	var req struct {
		Models []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供待删除模型列表"})
		return
	}
	models := make([]string, 0, len(req.Models))
	seen := make(map[string]bool)
	for _, m := range req.Models {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			models = append(models, m)
			seen[m] = true
		}
	}
	if len(models) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供待删除模型列表"})
		return
	}
	h.deleteModelRoutes(c, models)
}

func (h *AdminHandler) deleteModelRoutes(c *gin.Context, models []string) {
	if err := h.repo.DeleteModelRoutes(c.Request.Context(), models); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除模型路由失败: " + err.Error()})
		return
	}
	if h.sync != nil {
		if err := h.sync.ReloadAndBroadcast(c.Request.Context(), "model_routes_deleted"); err != nil {
			writeHotReloadWarning(c, &HotReloadError{Stage: "模型路由删除", Err: err}, nil)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已删除 %d 个模型路由，渠道模型配置已保留", len(models))})
}
