package controlplane

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// unbindModelsFromChannels disassociates a set of models from all channels and removes their fallback entries.
func unbindModelsFromChannels(repo *storage.Repository, modelSet map[string]bool) error {
	if repo == nil || len(modelSet) == 0 {
		return nil
	}

	allChannels, err := repo.ListChannels()
	if err == nil {
		for _, ch := range allChannels {
			changed := false
			var newModels []string
			for _, m := range ch.Models {
				if modelSet[m] {
					changed = true
				} else {
					newModels = append(newModels, m)
				}
			}
			if changed {
				ch.Models = newModels
			}
			if ch.ModelMapping != nil {
				for m := range modelSet {
					if _, ok := ch.ModelMapping[m]; ok {
						delete(ch.ModelMapping, m)
						changed = true
					}
				}
			}
			if changed {
				_ = repo.UpdateChannel(ch)
			}
		}
	}

	for m := range modelSet {
		_ = repo.DeleteModelFallback(m)
	}
	return nil
}

// DeleteModelRoute removes a model from all upstream channels and fallback definitions.
func (h *AdminHandler) DeleteModelRoute(c *gin.Context) {
	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模型标识为必填项"})
		return
	}

	_ = unbindModelsFromChannels(h.repo, map[string]bool{modelName: true})

	if err := h.sync.ReloadAndBroadcast(c.Request.Context(), "model_route_deleted"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据面热重载失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型路由及上下游绑定已成功删除"})
}

// BatchDeleteModelRoutes deletes routes for multiple models.
func (h *AdminHandler) BatchDeleteModelRoutes(c *gin.Context) {
	var req struct {
		Models []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Models) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供待删除模型列表"})
		return
	}

	modelSet := make(map[string]bool)
	for _, m := range req.Models {
		t := strings.TrimSpace(m)
		if t != "" {
			modelSet[t] = true
		}
	}

	_ = unbindModelsFromChannels(h.repo, modelSet)

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "models_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个模型路由及其服务商绑定", len(modelSet))})
}
