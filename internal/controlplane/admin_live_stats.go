package controlplane

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// GetLiveStats serves GET /api/v1/admin/stats/live: the in-flight telemetry the
// dashboard polls for real-time throughput. The payload is assembled from
// process memory plus, when Redis is active, the per-node snapshots peers
// publish every second. It never queries the database and never caches a
// response; both would defeat the point of a live view.
func (h *AdminHandler) GetLiveStats(c *gin.Context) {
	// These gauges are process/cluster-wide and carry no API-key dimension, so a
	// regular tenant cannot be shown a meaningful (or safe) slice of them: the
	// numbers would describe other tenants' traffic. Returning a null payload is
	// the documented fallback — the dashboard then keeps its key-scoped 60s view.
	if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
		return
	}

	local := telemetry.GlobalLiveMetrics.Snapshot()

	// Bound the optional Redis read: a slow or unreachable cluster must never
	// stall the dashboard, so aggregation degrades to this node's local values.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 250*time.Millisecond)
	defer cancel()

	data := distributed.AggregateLiveSnapshot(ctx, local)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
