package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// D-10: every discovery surface must advertise the same MCP revision set. /v1
// used to hardcode 2024-11-05 while /mcp negotiated up to 2026-07-28.
func TestV1InfoAdvertisesCurrentMCPVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1", nil)

	NewHandler(nil).HandleV1Info(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	newest := mcpSupportedVersions[0]
	oldest := mcpSupportedVersions[len(mcpSupportedVersions)-1]
	for _, want := range []string{newest, oldest, "2024-11-05"} {
		if !strings.Contains(body, want) {
			t.Errorf("/v1 discovery does not mention MCP %s: %s", want, body)
		}
	}
	if mcpDefaultProtocolVersion != newest {
		t.Errorf("default version %q is not the newest supported %q", mcpDefaultProtocolVersion, newest)
	}
}

// The /mcp server block and the /v1 protocol list are built from the same table,
// so a future revision only has to be added once.
func TestMCPInfoMatchesSupportedVersions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/mcp", nil)

	NewMCPHandler(nil, nil).HandleMCPInfo(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, version := range mcpSupportedVersions {
		if !strings.Contains(body, version) {
			t.Errorf("/mcp discovery does not advertise supported version %s", version)
		}
	}
	if !strings.Contains(body, mcpDefaultProtocolVersion) {
		t.Errorf("/mcp discovery does not advertise the default version %q", mcpDefaultProtocolVersion)
	}
}
