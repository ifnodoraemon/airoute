package distributed

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const redisProbeSecret = "SuperSecretPassw0rd"

// Regression: the parse-failure warning used to log the raw REDIS_URL, which
// carries the password in the userinfo section.
func TestInitRedisParseFailureDoesNotLogPassword(t *testing.T) {
	telemetry.ResetWarnGate()
	// Invalid scheme => redis.ParseURL fails and the warning branch runs.
	InitRedis("http://:" + redisProbeSecret + "@127.0.0.1:6379/0")
	t.Cleanup(func() { InitRedis("") })

	for _, entry := range telemetry.GlobalRingBuffer.GetRecentLogs(200, "", "", "", "") {
		if !strings.Contains(entry.Message, "REDIS_URL") {
			continue
		}
		rendered := fmt.Sprintf("%v", entry.Attrs)
		if strings.Contains(rendered, redisProbeSecret) {
			t.Fatalf("redis password leaked into the log entry: %s", rendered)
		}
		if !strings.Contains(rendered, "REDACTED") {
			t.Fatalf("expected the logged URL to be masked, got %s", rendered)
		}
	}
}

func TestRedactRedisURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"password in userinfo", "redis://:s3cret@cache:6379/0", "redis://:REDACTED@cache:6379/0"},
		{"user and password", "redis://app:s3cret@cache:6379/0", "redis://app:REDACTED@cache:6379/0"},
		{"username only", "redis://app@cache:6379", "redis://app@cache:6379"},
		{"no credentials", "redis://cache:6379/0", "redis://cache:6379/0"},
		{"unparsable with credentials", "redis://:s3cret@[::1", "redis://REDACTED@[::1"},
		{"unparsable without credentials", "not a url", "not a url"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactRedisURL(tc.in)
			if got != tc.want {
				t.Fatalf("redactRedisURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "s3cret") {
				t.Fatalf("password survived redaction: %q", got)
			}
		})
	}
}
