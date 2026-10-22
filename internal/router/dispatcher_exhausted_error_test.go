package router

import (
	"context"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// newOpenBreakerDispatcher builds a dispatcher whose only candidate is already
// circuit-broken, so a dispatch skips every channel without calling upstream.
func newOpenBreakerDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "primary", Type: model.ProviderOpenAI, BaseURL: "http://127.0.0.1:1", Models: []string{"only-model"}},
	})
	for i := 0; i < 3; i++ {
		d.circuitBreaker.RecordFailure("primary")
	}
	return d
}

// assertReadableBreakerError rejects the "%!w(<nil>)" formatting that used to
// reach the audit log when every candidate was skipped by an open breaker: no
// upstream is called in that path, so the loop ends with a nil last error and
// wrapping it told operators nothing.
func assertReadableBreakerError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected the dispatch to fail while the breaker is open")
	}
	msg := err.Error()
	if strings.Contains(msg, "%!w") || strings.Contains(msg, "<nil>") {
		t.Fatalf("unformatted nil error leaked into the audit message: %q", msg)
	}
	if !strings.Contains(msg, "circuit breaker") {
		t.Fatalf("message must name the open circuit breaker, got %q", msg)
	}
	if !strings.Contains(msg, "primary") {
		t.Fatalf("message must name the skipped channel, got %q", msg)
	}
}

func TestDispatchExhaustedByOpenBreakerExplainsItself(t *testing.T) {
	d := newOpenBreakerDispatcher(t)

	resp, err := d.Dispatch(context.Background(), &model.ChatCompletionRequest{Model: "only-model"})
	if err == nil {
		t.Fatalf("expected the dispatch to fail while the breaker is open, got %+v", resp)
	}
	assertReadableBreakerError(t, err)
}

func TestDispatchStreamExhaustedByOpenBreakerExplainsItself(t *testing.T) {
	d := newOpenBreakerDispatcher(t)

	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "only-model"})
	if err == nil {
		t.Fatalf("expected the stream dispatch to fail while the breaker is open, got %+v", stream)
	}
	assertReadableBreakerError(t, err)
}
