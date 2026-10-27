package tests

import (
	"os"
	"testing"
)

// The end-to-end tests create requests with httptest.NewRequest, whose default
// RemoteAddr is a public documentation address. The control-plane access guard
// is production behavior; opt the generic suite out here. The guard itself is
// covered by internal/middleware/admin_access_test.go and
// internal/api/admin_guard_wiring_test.go.
func TestMain(m *testing.M) {
	_ = os.Setenv("GATEWAY_ADMIN_ALLOW_PUBLIC", "true")
	os.Exit(m.Run())
}
