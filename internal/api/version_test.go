package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/buildinfo"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/stretchr/testify/require"
)

func TestHealthAndDiscoveryReportBuildVersion(t *testing.T) {
	engine := SetupRouter(router.NewDispatcher(nil), nil)
	for _, path := range []string{"/health", "/v1", "/v1beta"} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Version string `json:"version"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, buildinfo.Version, body.Version, path)
	}
}
