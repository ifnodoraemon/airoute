package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/router"
)

// Most API tests build requests with httptest.NewRequest, whose default
// RemoteAddr is 192.0.2.1 (the RFC 5737 documentation range, i.e. public). The
// control-plane guard is production behavior; opt the generic suite out and
// cover the guard explicitly in TestAdminAccessGuardIsWired below.
func TestMain(m *testing.M) {
	_ = os.Setenv("GATEWAY_ADMIN_ALLOW_PUBLIC", "true")
	os.Exit(m.Run())
}

// TestAdminAccessGuardIsWired proves the guard is registered before the admin
// API and the embedded console: a public client is rejected with 403, while a
// loopback client reaches the normal handlers.
func TestAdminAccessGuardIsWired(t *testing.T) {
	t.Setenv("GATEWAY_ADMIN_ALLOW_PUBLIC", "")
	t.Setenv("GATEWAY_ADMIN_ALLOWED_IPS", "")
	t.Setenv("GATEWAY_ADMIN_ALLOWED_HOSTS", "")
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

	engine := SetupRouter(router.NewDispatcher(nil), nil)

	call := func(path, remote string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = remote
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}

	if got := call("/app/", "8.8.8.8:1234"); got != http.StatusForbidden {
		t.Fatalf("public console = %d, want 403", got)
	}
	if got := call("/api/v1/admin/keys", "8.8.8.8:1234"); got != http.StatusForbidden {
		t.Fatalf("public admin API = %d, want 403", got)
	}
	if got := call("/app/", "127.0.0.1:1234"); got != http.StatusOK {
		t.Fatalf("loopback console = %d, want 200", got)
	}
	if got := call("/v1/models", "8.8.8.8:1234"); got == http.StatusForbidden {
		t.Fatal("the data plane must not be covered by the control-plane guard")
	}
}
