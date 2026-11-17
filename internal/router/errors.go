package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// DispatchError is the terminal failure of a dispatch attempt: either routing
// found no candidate channel for the model/protocol, or every candidate that
// was called failed.
//
// A served request reports the channel chain that carried it in
// Response.Channel. This type is the failure-path counterpart: it carries the
// same routing facts (which channels were actually called, and whether routing
// had a candidate at all) as data, so the audit log never has to recover them by
// looking for channel names inside Error() text.
//
// That distinction matters: Error() is human-facing prose. A message such as
// "no upstream provider available for requested model 'x'" contains no channel
// at all, yet a text scan will happily report the English word "provider" as
// one.
type DispatchError struct {
	Model     string
	Trail     string // attempted chain, or skipped channels when no upstream was called
	Attempted []string
	Skipped   []string
	NoRoute   bool // true when routing produced no candidate channel
	// PromptTokens is the estimated prompt size when the failure is a client
	// cancellation: the request already reached an upstream, so the provider has
	// consumed (and will bill us for) the prompt even though we got nothing back.
	// Zero when unknown.
	PromptTokens int
	// Canceled marks a request the caller abandoned (or whose own deadline
	// expired) rather than a routing outcome. Cross-model fallback must not fire
	// for it: replaying a canceled request on another model only adds upstream
	// load for a client that already hung up.
	Canceled   bool
	contextErr error

	Err error
}

// Error implements the error interface, preserving the underlying message.
func (e *DispatchError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying cause to errors.Is/errors.As.
func (e *DispatchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is preserves cancellation classification when a custom context cause is used.
func (e *DispatchError) Is(target error) bool {
	return e != nil && errors.Is(e.contextErr, target)
}

// AsDispatchError extracts a *DispatchError from an error chain.
func AsDispatchError(err error) (*DispatchError, bool) {
	var de *DispatchError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

// newNoRouteError builds the terminal error for a model/protocol that routing
// could not map to any channel. The trail still carries the channels attempted
// by earlier hops of a cross-model fallback chain, and is empty when the request
// never reached a channel.
func newNoRouteError(ctx context.Context, modelName string, err error) error {
	de := newDispatchError(ctx, modelName, nil, nil, err)
	de.NoRoute = true
	return de
}

// newDispatchError snapshots both routing trails across model fallback hops.
func newDispatchError(ctx context.Context, modelName string, attempted, skipped []string, err error) *DispatchError {
	trail := getRoutingTrail(withChannelTrail(ctx, attempted, skipped))
	return &DispatchError{
		Model:     modelName,
		Trail:     joinChannelTrail(exhaustedTrail(trail.attempted, trail.skipped)),
		Attempted: trail.attempted,
		Skipped:   trail.skipped,
		Err:       err,
	}
}

func newExhaustedError(ctx context.Context, modelName string, attempted, skipped []string, err error) error {
	return newDispatchError(ctx, modelName, attempted, skipped, err)
}

// newCanceledError attaches routing context to a request cancellation.
//
// When the downstream goes away, the dispatcher used to return the bare
// ctx.Err(). That error carries no channel, so the audit row had an empty
// channel column and an operator could not tell which upstream the request was
// waiting on when the client gave up. The DispatchError keeps errors.Is(err,
// context.Canceled) working (via Unwrap) while recording the in-flight channel.
func newCanceledError(ctx context.Context, attempted []string, channel string, promptTokens int) error {
	cause := context.Canceled
	if ctx != nil && ctx.Err() != nil {
		cause = context.Cause(ctx)
	}
	if channel != "" {
		attempted = append(append([]string(nil), attempted...), channel)
	}
	de := newDispatchError(ctx, "", attempted, nil, cause)
	de.PromptTokens, de.Canceled = promptTokens, true
	if ctx != nil {
		de.contextErr = ctx.Err()
	}
	return de
}

func stoppedDispatchError(ctx context.Context, modelName string, attempted, skipped []string, channel string, promptTokens int, lastErr error) error {
	ctx = withChannelTrail(ctx, nil, skipped)
	if requestBudgetExceeded(ctx, ctx.Err()) {
		if channel != "" {
			attempted = append(append([]string(nil), attempted...), channel)
		}
		return newDispatchError(ctx, modelName, attempted, nil, newRequestBudgetError(modelName, attempted, lastErr))
	}
	de := newCanceledError(ctx, attempted, channel, promptTokens).(*DispatchError)
	de.Model = modelName
	return de
}

// exhaustedTrail names the channels involved when a dispatch runs out of
// candidates: normally the ones actually called. When the circuit breaker
// bypassed every candidate, nothing was "attempted", and the skipped channels
// are the only attribution an operator can act on.
func exhaustedTrail(attempted, skipped []string) []string {
	if len(attempted) > 0 {
		return attempted
	}
	return skipped
}

// skippedOnlyError explains an exhausted dispatch in which every candidate was
// bypassed by the circuit breaker, so no upstream was actually called and the
// loop ended with a nil last error. Wrapping that nil error rendered as
// "%!w(<nil>)" in the audit log, which told the operator nothing; naming the
// real condition does.
func skippedOnlyError(modelName string, skipped []string) error {
	if len(skipped) == 0 {
		return fmt.Errorf("every candidate channel for model %s is unavailable; no upstream was called", modelName)
	}
	return fmt.Errorf("every candidate channel for model %s is temporarily disabled by the circuit breaker, so no upstream was called (skipped: %s)", modelName, strings.Join(skipped, ", "))
}
