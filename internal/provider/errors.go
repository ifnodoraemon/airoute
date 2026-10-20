package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// UpstreamError describes a non-2xx response from an upstream provider while
// preserving the HTTP status code. The router uses the status to tell genuine
// channel failures apart from client mistakes: a 400/404/422 caused by the
// caller's payload must not trip the circuit breaker, must not be retried on
// every other channel, and must not trigger a cross-model fallback.
type UpstreamError struct {
	Channel string
	Status  int
	Body    string
}

// Error implements the error interface.
func (e *UpstreamError) Error() string {
	if e == nil {
		return ""
	}
	body := e.Body
	if len(body) > 2048 {
		body = body[:2048]
	}
	if e.Channel != "" {
		return fmt.Sprintf("upstream %s returned status %d: %s", e.Channel, e.Status, body)
	}
	return fmt.Sprintf("upstream returned status %d: %s", e.Status, body)
}

// StatusCode returns the upstream HTTP status code.
func (e *UpstreamError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.Status
}

// NewUpstreamError builds an UpstreamError for a non-2xx upstream response.
func NewUpstreamError(channel string, status int, body []byte) *UpstreamError {
	return &UpstreamError{Channel: channel, Status: status, Body: string(body)}
}

// AsUpstreamError extracts an *UpstreamError from an error chain.
func AsUpstreamError(err error) (*UpstreamError, bool) {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue, true
	}
	return nil, false
}

// IsChannelFailure reports whether err indicates that the *channel* is unhealthy
// (and therefore belongs in the circuit-breaker / fallback accounting).
//
//   - network / DNS / TLS errors, timeouts and context deadline exceeded: yes
//   - 5xx and 429: yes (upstream overloaded or broken)
//   - 401/403: yes, the channel credential is broken
//   - other 4xx (400/404/413/422...): no, the caller's request is at fault
func IsChannelFailure(err error) bool {
	if err == nil {
		return false
	}
	if ue, ok := AsUpstreamError(err); ok {
		switch {
		case ue.Status == 401 || ue.Status == 403:
			return true
		case ue.Status == 408 || ue.Status == 429:
			return true
		case ue.Status >= 500:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Unclassified transport errors (EOF, connection reset, url.Error wrapping
	// a dial failure) are channel problems.
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"connection refused", "connection reset", "no such host", "i/o timeout",
		"tls handshake", "broken pipe", "eof", "dial tcp", "server misbehaving",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// IsClientError reports whether err is a deterministic 4xx rejection that would
// fail identically on every other channel. Callers can stop the fallback loop
// early instead of replaying a bad request across the whole fleet.
func IsClientError(err error) bool {
	ue, ok := AsUpstreamError(err)
	if !ok {
		return false
	}
	return ue.Status >= 400 && ue.Status < 500 && ue.Status != 401 && ue.Status != 403 && ue.Status != 408 && ue.Status != 429
}
