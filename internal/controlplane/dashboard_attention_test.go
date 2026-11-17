package controlplane

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestDashboardTimeSeriesAddsAdminChannelHealth(t *testing.T) {
	t.Setenv("GATEWAY_BREAKER_ENABLED", "true")
	d := router.NewDispatcher([]model.ChannelConfig{
		{Name: "failing", Status: "active"},
		{Name: "ready", Status: "active"},
		{Name: "disabled", Status: "disabled"},
	})
	for i := 0; i < 3; i++ {
		d.NoteChannelFailure("failing")
	}
	h := &AdminHandler{dispatcher: d}
	data := storage.NewEmptyTimeSeriesData("1h")
	for _, role := range []string{"admin", "user"} {
		t.Run(role, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("admin_claims", &AdminClaims{Role: role})
			encoded, err := json.Marshal(h.dashboardTimeSeries(c, data))
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal(encoded, &result))
			if role == "admin" {
				require.EqualValues(t, 1, result["tripped_channels"])
				require.EqualValues(t, 1, result["disabled_channels"])
			} else {
				require.NotContains(t, result, "tripped_channels")
				require.NotContains(t, result, "disabled_channels")
			}
			require.Contains(t, result["attention"], "failed_requests")
		})
	}
	require.Zero(t, data.Attention.FailedRequests)
}
