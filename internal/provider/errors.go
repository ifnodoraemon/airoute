package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// UpstreamError describes a non-2xx response from an upstream provider while
// preserving the HTTP status code. The router uses the status to tell genuine
// channel failures apart from client mistakes: a 400/413/422 caused by the
// caller's payload must not trip the circuit breaker, must not be retried on
// every other channel, and must not trigger a cross-model fallback. A 404 is
// different: it only says this node does not host the model, so the next
// candidate channel may still serve it.
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
	// Upstream bodies often end with a newline; trailing whitespace rendered as a
	// blank line in the audit table and made one-line messages look truncated.
	body = strings.TrimSpace(body)
	channel := ""
	if e.Channel != "" {
		channel = " " + e.Channel
	}
	// An empty body used to render a dangling "status 404: " — noise that read
	// like a truncated message. Say only what is known.
	if body == "" {
		return fmt.Sprintf("upstream%s returned status %d", channel, e.Status)
	}
	return fmt.Sprintf("upstream%s returned status %d: %s", channel, e.Status, body)
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
//   - 5xx, 401, 403, 408: yes (upstream broken or credential broken)
//   - 429: NO. A rate limit is backpressure from a *healthy* upstream; it swaps
//     to the next channel without tripping the breaker, otherwise three
//     rate-limit replies would open the circuit and turn backpressure into a
//     gateway 502 for the whole cooldown.
//   - other 4xx (400/404/413/422...): no, the caller's request is at fault or the
//     node simply does not host the model
func IsChannelFailure(err error) bool {
	if err == nil {
		return false
	}
	if ue, ok := AsUpstreamError(err); ok {
		switch {
		case ue.Status == 401 || ue.Status == 403:
			return true
		case ue.Status == 408:
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

// IsModelNotFound reports whether err is a 404 from a single upstream node.
//
// A 404 means "this node does not host the model", not "the request is wrong":
// the same request can succeed on a different node, so dispatch keeps fanning
// out to the next candidate channel instead of failing the request.
func IsModelNotFound(err error) bool {
	ue, ok := AsUpstreamError(err)
	return ok && ue.Status == 404
}

// IsFatalPayloadError reports whether err is a deterministic client payload
// error (400/413/422) that would fail identically on every channel, so the
// dispatcher may stop and surface it verbatim. 404 is deliberately excluded; see
// IsModelNotFound.
//
// This pair replaces the former IsClientError predicate, which lumped 400/413/422
// together with 404. The two need opposite actions (abort the fan-out vs try the
// next node), so a single "is 4xx" answer invited exactly the bug B-03 reported.
func IsFatalPayloadError(err error) bool {
	ue, ok := AsUpstreamError(err)
	if !ok {
		return false
	}
	return ue.Status == 400 || ue.Status == 413 || ue.Status == 422
}

// streamErrorStatus maps an upstream stream-error frame onto the HTTP status its
// unary equivalent would have returned.
//
// Providers report mid-stream failures (overload, quota, model errors) as an
// error object inside a 200 SSE stream, which is why they used to be plain
// fmt.Errorf values: the router's health accounting is status-driven, so those
// frames could not be classified and a channel that aborted every long stream
// stayed healthy forever (B-02). Carrying the status keeps the shared
// IsChannelFailure semantics intact: a mid-stream rate limit is still
// backpressure, a mid-stream overload is still a channel failure.
//
// An error frame that aborts an otherwise successful stream is upstream-side by
// definition, so unknown kinds fall back to 503.
func streamErrorStatus(code int, kinds ...string) int {
	if code >= 400 && code < 600 {
		return code
	}
	for _, kind := range kinds {
		k := strings.ToLower(kind)
		switch {
		case strings.Contains(k, "rate_limit"), strings.Contains(k, "ratelimit"),
			strings.Contains(k, "quota"), strings.Contains(k, "resource_exhausted"):
			return 429
		case strings.Contains(k, "overload"), strings.Contains(k, "unavailable"),
			strings.Contains(k, "server_error"), strings.Contains(k, "api_error"),
			strings.Contains(k, "internal"):
			return 503
		case strings.Contains(k, "invalid_request"), strings.Contains(k, "invalid_argument"):
			return 400
		case strings.Contains(k, "authentication"), strings.Contains(k, "permission"),
			strings.Contains(k, "unauthenticated"):
			return 401
		}
	}
	return 503
}

// jsonStatusCode narrows a JSON-decoded status/code field (number or string) to
// an int, returning 0 when it is absent or not a usable status.
func jsonStatusCode(value any) int {
	switch code := value.(type) {
	case float64:
		return int(code)
	case int:
		return code
	case string:
		if n, err := strconv.Atoi(code); err == nil {
			return n
		}
	}
	return 0
}
