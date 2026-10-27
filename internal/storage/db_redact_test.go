package storage

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const dsnProbeSecret = "DsnSecretPassw0rd"

// Regression: the PostgreSQL connection log kept everything after the last "@",
// so a libpq keyword/value DSN (which has no "@") was logged in full, password
// included.
func TestOpenDBLogDoesNotLeakDSNPassword(t *testing.T) {
	telemetry.ResetWarnGate()

	keywordDSN := "host=127.0.0.1 port=1 dbname=airoute user=airoute password=" + dsnProbeSecret + " sslmode=disable connect_timeout=1"
	urlDSN := "postgres://airoute:" + dsnProbeSecret + "@127.0.0.1:1/airoute?sslmode=disable&connect_timeout=1"
	for _, dsn := range []string{keywordDSN, urlDSN} {
		// The connection is expected to fail (port 1); only the log line matters.
		if _, err := OpenDB(dsn); err == nil {
			t.Fatalf("expected OpenDB to fail for %q", dsn)
		}
	}

	seen := false
	for _, entry := range telemetry.GlobalRingBuffer.GetRecentLogs(200, "", "", "", "") {
		if !strings.Contains(entry.Message, "initializing distributed PostgreSQL connection") {
			continue
		}
		seen = true
		rendered := fmt.Sprintf("%v", entry.Attrs)
		if strings.Contains(rendered, dsnProbeSecret) {
			t.Fatalf("postgres password leaked into the log entry: %s", rendered)
		}
	}
	if !seen {
		t.Fatal("expected the PostgreSQL connection log line to be emitted")
	}
}

func TestRedactDSN(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url form with password",
			in:   "postgres://airoute:s3cret@db:5432/airoute?sslmode=disable",
			want: "postgres://airoute:REDACTED@db:5432/airoute?sslmode=disable",
		},
		{
			name: "url form without password",
			in:   "postgres://airoute@db:5432/airoute",
			want: "postgres://airoute@db:5432/airoute",
		},
		{
			name: "url form with password query parameter",
			in:   "postgres://db:5432/airoute?password=s3cret&sslmode=disable",
			want: "postgres://db:5432/airoute?password=REDACTED&sslmode=disable",
		},
		{
			name: "keyword form",
			in:   "host=db port=5432 dbname=airoute user=airoute password=s3cret sslmode=disable",
			want: "host=db port=5432 dbname=airoute user=airoute password=REDACTED sslmode=disable",
		},
		{
			name: "keyword form with quoted password",
			in:   "host=db dbname=airoute password='s3 cret' sslmode=disable",
			want: "host=db dbname=airoute password=REDACTED sslmode=disable",
		},
		{
			name: "keyword form with sslpassword",
			in:   "host=db dbname=airoute sslpassword=s3cret",
			want: "host=db dbname=airoute sslpassword=REDACTED",
		},
		{
			name: "keyword form without password",
			in:   "host=db dbname=airoute user=airoute",
			want: "host=db dbname=airoute user=airoute",
		},
		{
			name: "sqlite path untouched",
			in:   "data/gateway.db",
			want: "data/gateway.db",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactDSN(tc.in)
			if got != tc.want {
				t.Fatalf("redactDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "s3cret") || strings.Contains(got, "s3 cret") {
				t.Fatalf("password survived redaction: %q", got)
			}
		})
	}
}
