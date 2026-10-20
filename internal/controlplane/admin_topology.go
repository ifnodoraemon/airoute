package controlplane

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func (h *AdminHandler) GetRoutingTopology(c *gin.Context) {
	if _, ok := RequireAdminClaims(c, "仅管理员可查看运行时容灾拓扑"); !ok {
		return
	}
	if h.dispatcher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "调度器尚未就绪"})
		return
	}
	snapshot := h.dispatcher.GetRoutingSnapshot(strings.TrimSpace(c.Query("model")))
	now := time.Now()
	node, _ := os.Hostname()
	var activity *storage.RoutingActivity
	var activityError string
	if snapshot.Model != "" {
		var err error
		activity, err = h.repo.GetRoutingActivity(snapshot.Model, now.Add(-24*time.Hour))
		if err != nil {
			activityError = "调用日志统计暂时不可用"
		}
	}
	var version uint64
	if h.sync != nil {
		version = h.sync.Version()
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"routing": snapshot, "node": node, "config_version": version,
		"observed_at": now, "activity": activity, "activity_error": activityError,
	}})
}
