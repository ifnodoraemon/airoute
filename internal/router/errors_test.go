package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// A model with no route at all is a routing fact, and it must reach callers as
// data: the audit log records the channel chain from DispatchError, never by
// scanning the message text (that is how the literal word "provider" once ended
// up in the channel column).
func TestDispatchNoRouteReportsRoutingFacts(t *testing.T) {
	d := NewDispatcher(nil)

	_, err := d.Dispatch(context.Background(), &model.ChatCompletionRequest{Model: "llm-rerank-shim"})
	if err == nil {
		t.Fatal("expected an error for a model with no candidate channel")
	}

	de, ok := AsDispatchError(err)
	if !ok {
		t.Fatalf("expected *DispatchError, got %T: %v", err, err)
	}
	if !de.NoRoute {
		t.Errorf("NoRoute = false, want true")
	}
	if de.Model != "llm-rerank-shim" {
		t.Errorf("Model = %q, want %q", de.Model, "llm-rerank-shim")
	}
	if de.Trail != "" {
		t.Errorf("Trail = %q, want empty: no channel was ever reached", de.Trail)
	}

	// The human-facing message is part of the API contract (clients and log
	// greps match it); carrying routing facts must not rewrite it.
	want := "no upstream provider available for requested model 'llm-rerank-shim'"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error() = %q, want it to contain %q", err.Error(), want)
	}
}

func TestDispatchErrorSurvivesWrapping(t *testing.T) {
	base := newExhaustedError(context.Background(), "gpt-4o", []string{"ch-a", "ch-b"}, errors.New("boom"))
	wrapped := fmt.Errorf("handler: %w", base)

	de, ok := AsDispatchError(wrapped)
	if !ok {
		t.Fatalf("AsDispatchError failed on a wrapped DispatchError: %v", wrapped)
	}
	if de.Trail != "ch-a→ch-b" {
		t.Errorf("Trail = %q, want %q", de.Trail, "ch-a→ch-b")
	}
	if de.NoRoute {
		t.Error("NoRoute = true, want false for an exhausted dispatch")
	}
	if got := de.Error(); got != "boom" {
		t.Errorf("Error() = %q, want the underlying message %q", got, "boom")
	}
	if !errors.Is(wrapped, de.Unwrap()) {
		t.Error("errors.Is did not find the underlying cause through DispatchError")
	}
}

func TestFormatAttemptedTrail(t *testing.T) {
	t.Run("no hop at all", func(t *testing.T) {
		if got := formatAttemptedTrail(context.Background(), nil); got != "" {
			t.Errorf("formatAttemptedTrail = %q, want empty", got)
		}
	})

	t.Run("joins prior fallback hops and current attempts", func(t *testing.T) {
		ctx := withChannelTrail(context.Background(), []string{"ch-prior"})
		if got := formatAttemptedTrail(ctx, []string{"ch-a", "ch-b"}); got != "ch-prior→ch-a→ch-b" {
			t.Errorf("formatAttemptedTrail = %q, want %q", got, "ch-prior→ch-a→ch-b")
		}
	})

	t.Run("does not append a serving hop", func(t *testing.T) {
		if got := formatAttemptedTrail(context.Background(), []string{"ch-a"}); got != "ch-a" {
			t.Errorf("formatAttemptedTrail = %q, want %q", got, "ch-a")
		}
	})
}
