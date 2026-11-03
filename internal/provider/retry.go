package provider

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func retryHeaders(src http.Header) http.Header {
	out := make(http.Header)
	for _, name := range []string{"Retry-After", "X-RateLimit-Reset-Requests", "X-RateLimit-Reset-Tokens", "Anthropic-RateLimit-Requests-Reset", "Anthropic-RateLimit-Tokens-Reset", "X-RateLimit-Scope"} {
		if value := src.Get(name); value != "" {
			out.Set(name, value)
		}
	}
	return out
}

// RetryDelay reads standard Retry-After and provider reset timestamps. It is
// bounded so a malformed or hostile header cannot disable an upstream forever.
func RetryDelay(headers http.Header, now time.Time) time.Duration {
	var delay time.Duration
	if value := headers.Get("Retry-After"); value != "" {
		if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
			delay = time.Duration(min(seconds, 86400) * float64(time.Second))
		} else if at, err := http.ParseTime(value); err == nil {
			delay = at.Sub(now)
		}
	}
	for _, name := range []string{"X-RateLimit-Reset-Requests", "X-RateLimit-Reset-Tokens", "Anthropic-RateLimit-Requests-Reset", "Anthropic-RateLimit-Tokens-Reset"} {
		value := strings.TrimSpace(headers.Get(name))
		if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
			delay = max(delay, at.Sub(now))
		} else if d, err := time.ParseDuration(value); err == nil {
			delay = max(delay, d)
		} else if epoch, err := strconv.ParseInt(value, 10, 64); err == nil && epoch > 1e9 {
			delay = max(delay, time.Unix(epoch, 0).Sub(now))
		}
	}
	return min(max(delay, 0), 24*time.Hour)
}
