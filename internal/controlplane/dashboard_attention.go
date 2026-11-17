package controlplane

import (
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Keep runtime channel health outside the cached, key-scoped SQL result.
func (h *AdminHandler) dashboardTimeSeries(c *gin.Context, data *storage.TimeSeriesData) any {
	claims, ok := GetClaimsFromContext(c)
	if !ok || claims.Role != "admin" || h.dispatcher == nil {
		return data
	}
	tripped, disabled := h.dispatcher.ChannelHealthCounts()
	return struct {
		*storage.TimeSeriesData
		TrippedChannels  int `json:"tripped_channels"`
		DisabledChannels int `json:"disabled_channels"`
	}{data, tripped, disabled}
}
