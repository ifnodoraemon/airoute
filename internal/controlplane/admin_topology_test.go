package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestRoutingTopologyRequiresAdminAndUsesRuntimeConfiguration(t *testing.T) {
	handler, _ := newRouteTestHandler(t)
	handler.dispatcher = router.NewDispatcher([]model.ChannelConfig{{ID: 7, Name: "runtime-gpu", Type: model.ProviderGPUStack, Models: []string{"alias"}, APIKey: "top-secret", BaseURL: "https://private", Priority: 4}})
	for _, role := range []string{"", "user", "admin"} {
		t.Run(role, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/models/topology?model=alias", nil)
			if role != "" {
				c.Set("admin_claims", &AdminClaims{Role: role})
			}
			handler.GetRoutingTopology(c)
			want := http.StatusOK
			if role == "" {
				want = http.StatusUnauthorized
			} else if role == "user" {
				want = http.StatusForbidden
			}
			if w.Code != want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if role != "admin" {
				return
			}
			var body struct {
				Data struct {
					Routing router.RoutingSnapshot `json:"routing"`
					Node    string                 `json:"node"`
				}
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Data.Routing.Steps[0].Providers[0].Name != "runtime-gpu" || body.Data.Node == "" {
				t.Fatalf("runtime state missing: %s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), "top-secret") || strings.Contains(w.Body.String(), "https://private") {
				t.Fatal("credential in topology")
			}
		})
	}
}
