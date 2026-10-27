package provider

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultDirectCallTimeout is the backstop for provider calls that carry no
// request-context deadline. Dispatcher paths always supply the channel budget
// (channelCallContext), so this only covers auxiliary endpoints that talk to an
// upstream directly (for example the Anthropic count_tokens passthrough). It is
// deliberately far above any interactive request so it never becomes a hidden
// request budget; it only prevents an unbounded wait.
const DefaultDirectCallTimeout = 10 * time.Minute

// SharedDefaultHTTPClient reuses connections opened on demand across upstream requests.
//
// The transport deliberately carries no fixed response-header deadline. An
// upstream budget is a per-channel setting (channel.timeout_seconds, accepted up
// to model.MaxChannelTimeoutSeconds), and the router applies it to every provider
// call as a request-context deadline (channelCallContext). A hard-coded 60s
// ResponseHeaderTimeout used to override that setting: a channel configured for
// 300s was aborted at 60s, and because the abort surfaced as a net.Error timeout
// the circuit breaker charged the channel for a failure even though the upstream
// was only slow. The bound therefore follows the configured channel timeout
// instead of a transport constant.
//
// A call whose context has no deadline is still bounded by
// DefaultDirectCallTimeout (see deadlineGuardTransport) so removing the transport
// constant cannot leave any upstream call unbounded. ResponseHeaderTimeout is
// zero so no transport constant can override the request deadline; Timeout is
// zero so long SSE streams and long reasoning generations are governed by that
// single request-context deadline.
var SharedDefaultHTTPClient = &http.Client{
	Transport: &deadlineGuardTransport{base: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2048,
		MaxIdleConnsPerHost:   512,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}},
	Timeout: 0, // No client-level body deadline to allow long-running SSE streams; the request context is the budget.
}

// deadlineGuardTransport applies DefaultDirectCallTimeout only when the caller
// supplied no deadline of its own. Calls that carry the channel budget (every
// dispatcher path) are passed through untouched, so a configured 300s or 24h
// channel timeout is never shortened by the transport.
type deadlineGuardTransport struct {
	base http.RoundTripper
}

func (t *deadlineGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, ok := req.Context().Deadline(); ok {
		return t.base.RoundTrip(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), DefaultDirectCallTimeout)
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	// The response body owns the deadline: a streamed body must stay readable.
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelOnCloseBody releases a request-context deadline once the body is closed.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}
