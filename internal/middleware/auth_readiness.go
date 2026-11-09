package middleware

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// dataPlaneReady distinguishes "the gateway genuinely has no API keys" from
// "the runtime configuration has not been loaded yet".
//
// It defaults to true so embedders and tests that never wire the startup path
// keep working; cmd/airoute sets it to false before the first snapshot load and
// flips it back once storage answered. Without that distinction a failed first
// snapshot read leaves the in-memory key set empty, and the open-dev-mode
// branch below would admit every caller anonymously.
var dataPlaneReady atomic.Bool

func init() { dataPlaneReady.Store(true) }

// SetDataPlaneReady marks whether the runtime configuration has been loaded
// from durable storage at least once. It is called by the server bootstrap.
func SetDataPlaneReady(ready bool) { dataPlaneReady.Store(ready) }

// DataPlaneReady reports whether the data plane has a loaded configuration.
func DataPlaneReady() bool { return dataPlaneReady.Load() }

// DataPlaneUnreadyDenial is the fail-closed response served while the gateway
// has not loaded its configuration yet.
func DataPlaneUnreadyDenial() *AccessDenial {
	return &AccessDenial{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "Gateway data plane is not ready: configuration has not been loaded from storage yet. Please retry shortly.",
		Type:       "service_unavailable",
		Code:       "data_plane_unready",
		Headers:    map[string]string{"Retry-After": "5"},
		LogMessage: "data plane configuration not loaded yet",
	}
}

func admitDataPlaneReady(c *gin.Context) bool {
	if DataPlaneReady() {
		return true
	}
	denial := DataPlaneUnreadyDenial()
	auditAuthFailure(c, "", denial.StatusCode, denial.LogMessage)
	for header, value := range denial.Headers {
		c.Header(header, value)
	}
	AbortWithDenial(c, denial)
	return false
}
