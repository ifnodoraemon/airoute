package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// R-01: the shared transport used to hard-code ResponseHeaderTimeout=60s, which
// capped every channel timeout below that value (channels may be configured up to
// model.MaxChannelTimeoutSeconds) and charged the resulting timeout to the breaker
// as a channel failure. The budget must come from the request context instead.
func TestSharedClientHasNoFixedHeaderDeadline(t *testing.T) {
	base := SharedDefaultHTTPClient.Transport
	if guard, ok := base.(*deadlineGuardTransport); ok {
		base = guard.base
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", base)
	}
	if transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("transport ResponseHeaderTimeout = %v: a fixed transport deadline overrides the configured channel timeout", transport.ResponseHeaderTimeout)
	}
	if SharedDefaultHTTPClient.Timeout != 0 {
		t.Fatalf("client Timeout = %v: a client-wide deadline would cap long streams", SharedDefaultHTTPClient.Timeout)
	}
}

// captureRoundTripper records the deadline the guard handed to the transport.
type captureRoundTripper struct{ observed []time.Duration }

func (r *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if deadline, ok := req.Context().Deadline(); ok {
		r.observed = append(r.observed, time.Until(deadline))
	} else {
		r.observed = append(r.observed, 0)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

// The guard must never shorten a call that already carries the channel budget,
// and must bound the auxiliary calls that carry none.
func TestDeadlineGuardPreservesChannelBudget(t *testing.T) {
	capture := &captureRoundTripper{}
	guard := &deadlineGuardTransport{base: capture}

	budgetCtx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(budgetCtx, http.MethodPost, "https://upstream.invalid/x", nil)
	resp, err := guard.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip with channel budget: %v", err)
	}
	resp.Body.Close()

	bare, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://upstream.invalid/x", nil)
	resp, err = guard.RoundTrip(bare)
	if err != nil {
		t.Fatalf("round trip without budget: %v", err)
	}
	resp.Body.Close()

	if len(capture.observed) != 2 {
		t.Fatalf("observed %d deadlines", len(capture.observed))
	}
	if capture.observed[0] < 290*time.Second {
		t.Fatalf("guard replaced the 300s channel budget with %v", capture.observed[0])
	}
	if capture.observed[1] == 0 {
		t.Fatalf("a deadline-less direct call stayed unbounded")
	}
	if capture.observed[1] < DefaultDirectCallTimeout-5*time.Second {
		t.Fatalf("guard default = %v, want %v", capture.observed[1], DefaultDirectCallTimeout)
	}
}

// The request-context deadline is the only header/body bound. This is a fast
// behavioural check that the bound is honoured without a transport constant (the
// 60s regression needed a 60s wait to observe).
func TestProviderCallBoundedByContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(600 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(SharedDefaultHTTPClient)
	ch := &model.ChannelConfig{Name: "slow", Type: model.ProviderOpenAI, BaseURL: srv.URL, Models: []string{"m"}}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.ChatComplete(ctx, &model.ChatCompletionRequest{Model: "m"}, ch)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("call succeeded despite an expired 150ms budget")
	}
	if elapsed > time.Second {
		t.Fatalf("call returned after %v: the request-context budget was not the bound", elapsed)
	}
}

// A large configured channel budget must be honoured: the transport must not abort
// before the context deadline. 1.2s is enough to exceed any small transport
// constant that could be reintroduced, while staying fast.
func TestProviderCallHonoursLongerContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1200 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(SharedDefaultHTTPClient)
	ch := &model.ChannelConfig{Name: "slow-ok", Type: model.ProviderOpenAI, BaseURL: srv.URL, Models: []string{"m"}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := p.ChatComplete(ctx, &model.ChatCompletionRequest{Model: "m"}, ch)
	if err != nil {
		t.Fatalf("a call inside its configured budget failed: %v", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		t.Fatalf("unexpected response %+v", resp)
	}
}
