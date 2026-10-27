package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// R-06: advisory mode (GATEWAY_BREAKER_ENABLED=false, the shipped .env value) is
// documented as "records failures but never rejects traffic". It used to return
// before touching the status, so every channel stayed CLOSED/0 in the console.
func TestAdvisoryBreakerRecordsFailuresWithoutRejecting(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second)
	cb.SetDisabled(true)

	for i := 0; i < 3; i++ {
		cb.RecordFailure("ch")
	}
	snapshot := cb.Snapshot("ch")
	if snapshot.Failures != 3 {
		t.Fatalf("advisory breaker recorded %d failures, want 3: operators cannot see the outage", snapshot.Failures)
	}
	if snapshot.State != StateOpen.String() {
		t.Fatalf("advisory breaker state = %s, want %s so the failure is visible", snapshot.State, StateOpen)
	}
	if !snapshot.CanExecute {
		t.Fatalf("advisory snapshot claims the channel is unusable; advisory mode never rejects traffic")
	}
	if !cb.CanExecute("ch") {
		t.Fatalf("advisory breaker rejected traffic: it must only observe")
	}
	if !cb.Healthy("ch") {
		t.Fatalf("advisory breaker reported the channel unhealthy for routing decisions")
	}

	cb.RecordSuccess("ch")
	if snapshot := cb.Snapshot("ch"); snapshot.Failures != 0 || snapshot.State != StateClosed.String() {
		t.Fatalf("success did not clear the advisory state: %+v", snapshot)
	}
}

// The protective breaker must keep tripping exactly as before.
func TestEnabledBreakerStillTripsAndRejects(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second)
	for i := 0; i < 3; i++ {
		cb.RecordFailure("ch")
	}
	if cb.CanExecute("ch") {
		t.Fatal("enabled breaker did not open")
	}
	if snapshot := cb.Snapshot("ch"); snapshot.CanExecute || snapshot.State != StateOpen.String() {
		t.Fatalf("enabled breaker snapshot = %+v, want OPEN and not executable", snapshot)
	}
}

// R-03: an upstream 429 is backpressure from a healthy provider, exactly as in the
// chat path; it must not be charged to the circuit breaker.
func TestDispatchHTTPRateLimitDoesNotTripBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	d := NewDispatcher([]model.ChannelConfig{{
		Name: "img", Type: model.ProviderOpenAI, BaseURL: srv.URL, Models: []string{"img-model"},
		Protocols: []string{"images"}, Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 5,
	}})

	for i := 0; i < 3; i++ {
		_, err := d.DispatchHTTP(context.Background(), &UpstreamRequest{
			Path: "/v1/images/generations", Method: http.MethodPost,
			Body: []byte(`{"model":"img-model","prompt":"x"}`), ContentType: "application/json",
			Model: "img-model", Protocol: "images",
		})
		if err == nil {
			t.Fatalf("attempt %d unexpectedly succeeded", i+1)
		}
		if ue, ok := provider.AsUpstreamError(err); !ok || ue.Status != http.StatusTooManyRequests {
			t.Fatalf("attempt %d error = %v, want the upstream 429 surfaced", i+1, err)
		}
	}

	snapshot := d.circuitBreaker.Snapshot("img")
	if snapshot.Failures != 0 || snapshot.State != StateClosed.String() {
		t.Fatalf("three upstream 429s charged the breaker: %+v (rate limits must not open the circuit)", snapshot)
	}
}

// R-03 guard: genuine upstream failures must still be charged and open the circuit.
func TestDispatchHTTPServerErrorsStillTripBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	d := NewDispatcher([]model.ChannelConfig{{
		Name: "img", Type: model.ProviderOpenAI, BaseURL: srv.URL, Models: []string{"img-model"},
		Protocols: []string{"images"}, Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 5,
	}})

	for i := 0; i < 3; i++ {
		_, _ = d.DispatchHTTP(context.Background(), &UpstreamRequest{
			Path: "/v1/images/generations", Method: http.MethodPost,
			Body: []byte(`{"model":"img-model","prompt":"x"}`), ContentType: "application/json",
			Model: "img-model", Protocol: "images",
		})
	}
	snapshot := d.circuitBreaker.Snapshot("img")
	if snapshot.State != StateOpen.String() {
		t.Fatalf("three upstream 500s did not open the breaker: %+v", snapshot)
	}
}
