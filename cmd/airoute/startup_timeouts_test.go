package main

import (
	"strings"
	"testing"
	"time"
)

// The shutdown budget must cover the aborted-request drain window instead of the
// old hardcoded 10s (which was shorter than the 120s drain and raced Docker's
// default 10s SIGKILL). It is configurable and otherwise derived from the drain
// budget.
func TestShutdownTimeoutDerivation(t *testing.T) {
	cases := []struct {
		name        string
		shutdownEnv string
		drainEnv    string
		want        time.Duration
	}{
		{"explicit override", "45", "", 45 * time.Second},
		{"derived from drain", "", "200", 215 * time.Second},
		{"defaults", "", "", 135 * time.Second},
		{"invalid override falls back to drain", "not-a-number", "90", 105 * time.Second},
		{"invalid drain uses default", "", "not-a-number", 135 * time.Second},
		{"zero override ignored", "0", "", 135 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GATEWAY_SHUTDOWN_TIMEOUT_SEC", tc.shutdownEnv)
			t.Setenv("GATEWAY_DRAIN_TIMEOUT_SEC", tc.drainEnv)
			if got := shutdownTimeout(); got != tc.want {
				t.Fatalf("shutdownTimeout() = %s, want %s", got, tc.want)
			}
		})
	}
}

// The startup log must describe the store that is actually in use and must never
// print DATABASE_URL verbatim (it contains the database password).
func TestDescribeDataSourceRedactsCredentials(t *testing.T) {
	const dsn = "postgres://airoute:sup3r-s3cret@db.internal:5432/airoute?sslmode=disable"
	kind, target := describeDataSource(dsn)
	if kind != "postgres" {
		t.Fatalf("kind = %q, want postgres", kind)
	}
	if strings.Contains(target, "sup3r-s3cret") {
		t.Fatalf("redacted target still contains the password: %q", target)
	}
	if target != "db.internal:5432/airoute" {
		t.Fatalf("target = %q, want host/database only", target)
	}

	kind, target = describeDataSource("postgresql://user:pw@other-host:5432/otherdb")
	if kind != "postgres" || strings.Contains(target, "pw@") {
		t.Fatalf("postgresql:// URL handled incorrectly: kind=%q target=%q", kind, target)
	}

	kind, target = describeDataSource("data/gateway.db")
	if kind != "sqlite" || target != "data/gateway.db" {
		t.Fatalf("sqlite source = (%q, %q), want sqlite/data/gateway.db", kind, target)
	}
}
