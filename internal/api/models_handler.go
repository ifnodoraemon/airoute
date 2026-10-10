package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// HandleModels handles GET /v1/models.
func (h *Handler) HandleModels(c *gin.Context) {
	models := h.dispatcher.GetAllSupportedModels()
	items := make([]model.ModelItem, 0, len(models))
	now := time.Now().Unix()

	for _, m := range models {
		if !middleware.ValidateModelAllowed(c, m) {
			continue
		}
		items = append(items, model.ModelItem{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "airoute",
		})
	}

	c.JSON(http.StatusOK, model.ModelListResponse{
		Object: "list",
		Data:   items,
	})
}

// HandleModelDetail handles GET /v1/models/:model.
func (h *Handler) HandleModelDetail(c *gin.Context) {
	modelID := c.Param("model")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Model ID is required",
				"type":    "invalid_request_error",
				"code":    "missing_model_id",
			},
		})
		return
	}

	if !middleware.ValidateModelAllowed(c, modelID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Your API key does not have access to model '%s'.", modelID),
				"type":    "invalid_request_error",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	channels := h.dispatcher.GetChannelsForModel(modelID)
	if len(channels) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("The model '%s' does not exist or is not available.", modelID),
				"type":    "invalid_request_error",
				"code":    "model_not_found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, model.ModelItem{
		ID:      modelID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "airoute",
	})
}
