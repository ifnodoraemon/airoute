package controlplane

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// GetSupplierReconciliation handles supplier-side FinOps reconciliation queries.
func (h *AdminHandler) GetSupplierReconciliation(c *gin.Context) {
	filter := storage.ChannelReconciliationFilter{
		StartTime: c.Query("start_time"),
		EndTime:   c.Query("end_time"),
		Channel:   c.Query("channel"),
		Model:     c.Query("model"),
	}

	report, err := h.repo.GetSupplierReconciliation(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":  1,
			"error": "获取供应商对账报告失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": report,
	})
}
