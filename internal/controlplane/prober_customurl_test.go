package controlplane

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestProber builds a prober with a short timeout so an offline-host test does
// not wait for the production 15s budget.
func newTestProber() *DownstreamProber {
	return NewDownstreamProber(&http.Client{Timeout: 3 * time.Second})
}

// closedLoopbackURL returns a loopback URL nothing is listening on.
func closedLoopbackURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close loopback listener: %v", err)
	}
	return "http://" + addr
}

// allowLoopbackProbe clears the optional probe restrictions so the test can reach
// its own httptest server.
func allowLoopbackProbe(t *testing.T) {
	t.Helper()
	t.Setenv("GATEWAY_BLOCK_PRIVATE_PROBE", "false")
	t.Setenv("GATEWAY_BLOCK_LOOPBACK_PROBE", "false")
}

// probeRequestLog records what a fake downstream actually received. The httptest
// handler runs on its own goroutine, so the record is mutex-guarded.
type probeRequestLog struct {
	mu      sync.Mutex
	paths   []string
	apiKeys []string
	keys    []string
}

func (l *probeRequestLog) record(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = append(l.paths, r.URL.Path)
	l.apiKeys = append(l.apiKeys, r.Header.Get("x-api-key"))
	l.keys = append(l.keys, r.URL.Query().Get("key"))
}

func (l *probeRequestLog) hasPath(want string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, path := range l.paths {
		if path == want {
			return true
		}
	}
	return false
}

func (l *probeRequestLog) anyPathContains(fragment string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, path := range l.paths {
		if strings.Contains(path, fragment) {
			return true
		}
	}
	return false
}

// fullPathCall returns the recorded (path, x-api-key) pair for one path.
func (l *probeRequestLog) fullPathCall(want string) (string, string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, path := range l.paths {
		if path == want {
			return path, l.apiKeys[i], true
		}
	}
	return "", "", false
}

// queryKeyAt returns the ?key= query value recorded for one path.
func (l *probeRequestLog) queryKeyAt(want string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, path := range l.paths {
		if path == want {
			return l.keys[i], true
		}
	}
	return "", false
}

// #13: an offline host must fail the probe, not produce a success-looking result.
// The old fallback returned a ProbeResult with "已成功连通" wording for every
// failure, including connection refused, so the console offered a channel that
// could never serve traffic.
func TestProbeFailsWhenHostNeverAnswers(t *testing.T) {
	allowLoopbackProbe(t)

	_, err := newTestProber().Probe(context.Background(), &ProbeRequest{BaseURL: closedLoopbackURL(t)})
	if err == nil {
		t.Fatal("an unreachable host produced a ProbeResult instead of an error")
	}
	if !strings.Contains(err.Error(), "无法连接下游服务") {
		t.Errorf("error = %q, want it to name the unreachable host", err.Error())
	}
}

// #13: any HTTP response proves reachability. A 401/404 proxy is a warning result
// whose text must not claim success.
func TestProbeWarnsWhenHostAnswersButCatalogueIsUnavailable(t *testing.T) {
	allowLoopbackProbe(t)

	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound} {
		t.Run(fmt.Sprintf("HTTP %d", status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"synthetic"}`))
			}))
			defer srv.Close()

			result, err := newTestProber().Probe(context.Background(), &ProbeRequest{BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("a reachable host returned an error: %v", err)
			}
			if result == nil {
				t.Fatal("a reachable host returned no result")
			}
			if result.Message == "" {
				t.Error("warning result has no message")
			}
			if strings.Contains(result.Message, "成功") {
				t.Errorf("warning message claims success: %q", result.Message)
			}
			if !strings.Contains(result.Message, "已响应") {
				t.Errorf("warning message should say the host answered: %q", result.Message)
			}
			if !strings.Contains(result.Message, fmt.Sprint(status)) {
				t.Errorf("warning message should carry the observed status %d: %q", status, result.Message)
			}
		})
	}
}

// #14: Gemini probes must use the configured base URL, including a custom proxy,
// and must normalise a base URL that already ends in /v1beta.
func TestProbeGeminiUsesConfiguredBaseURL(t *testing.T) {
	allowLoopbackProbe(t)

	log := &probeRequestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		if r.URL.Path == "/v1beta/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.0-flash"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	result, err := newTestProber().Probe(context.Background(), &ProbeRequest{
		BaseURL: srv.URL + "/v1beta", Type: "gemini", APIKey: "synthetic-gemini-key",
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if len(result.Models) != 1 || result.Models[0] != "gemini-2.0-flash" {
		t.Fatalf("models = %v, want the custom proxy catalogue", result.Models)
	}
	if log.anyPathContains("/v1beta/v1beta") {
		t.Fatal("base URL was not normalised before the models request")
	}
	if _, _, ok := log.fullPathCall("/v1beta/models"); !ok {
		t.Fatal("custom proxy was never asked for /v1beta/models")
	}
	if got, ok := log.queryKeyAt("/v1beta/models"); !ok || got != "synthetic-gemini-key" {
		// Gemini authenticates with the query parameter, not a header.
		t.Errorf("?key = %q, want the supplied Gemini key", got)
	}
}

// #14: a keyless Gemini probe is still a real reachability request, so an offline
// custom proxy fails instead of returning a plausible-looking configuration.
func TestProbeGeminiKeylessStillTestsReachability(t *testing.T) {
	allowLoopbackProbe(t)

	log := &probeRequestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	result, err := newTestProber().Probe(context.Background(), &ProbeRequest{
		BaseURL: srv.URL, Type: "gemini",
	})
	if err != nil {
		t.Fatalf("reachable keyless proxy returned an error: %v", err)
	}
	if !log.hasPath("/v1beta/models") {
		t.Fatal("keyless probe never touched the custom proxy")
	}
	if strings.Contains(result.Message, "成功") {
		t.Errorf("keyless warning claims success: %q", result.Message)
	}

	if _, err := newTestProber().Probe(context.Background(), &ProbeRequest{
		BaseURL: closedLoopbackURL(t), Type: "gemini",
	}); err == nil {
		t.Fatal("keyless probe of an offline custom proxy returned a result instead of an error")
	}
}

// #14: Anthropic probes behave the same: configured host, normalised /v1, real
// request for the keyless path, transport failure is an error.
func TestProbeAnthropicUsesConfiguredBaseURL(t *testing.T) {
	allowLoopbackProbe(t)

	log := &probeRequestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-synthetic"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	result, err := newTestProber().Probe(context.Background(), &ProbeRequest{
		BaseURL: srv.URL + "/v1", Type: "anthropic", APIKey: "synthetic-anthropic-key",
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if len(result.Models) != 1 || result.Models[0] != "claude-synthetic" {
		t.Fatalf("models = %v, want the custom proxy catalogue", result.Models)
	}
	if log.anyPathContains("/v1/v1") {
		t.Fatal("base URL was not normalised before the models request")
	}
	if _, apiKey, ok := log.fullPathCall("/v1/models"); !ok {
		t.Fatal("custom proxy was never asked for /v1/models")
	} else if apiKey != "synthetic-anthropic-key" {
		t.Errorf("x-api-key = %q, want the supplied key", apiKey)
	}

	// Keyless: the custom host must still receive a request, and an offline host
	// must fail the probe.
	keyless, err := newTestProber().Probe(context.Background(), &ProbeRequest{BaseURL: srv.URL, Type: "anthropic"})
	if err != nil {
		t.Fatalf("reachable keyless Anthropic proxy returned an error: %v", err)
	}
	if strings.Contains(keyless.Message, "成功") {
		t.Errorf("keyless warning claims success: %q", keyless.Message)
	}
	if _, err := newTestProber().Probe(context.Background(), &ProbeRequest{
		BaseURL: closedLoopbackURL(t), Type: "anthropic",
	}); err == nil {
		t.Fatal("keyless probe of an offline Anthropic proxy returned a result instead of an error")
	}
}
