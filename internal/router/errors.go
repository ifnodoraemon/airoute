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
	Model   string
	Trail   string // channels actually called, joined with "→"; "" when none
	NoRoute bool   // true when routing produced no candidate channel
	// PromptTokens is the estimated prompt size when the failure is a client
	// cancellation: the request already reached an upstream, so the provider has
	// consumed (and will bill us for) the prompt even though we got nothing back.
	// Zero when unknown.
	PromptTokens int

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
	return &DispatchError{
		Model:   modelName,
		Trail:   formatAttemptedTrail(ctx, nil),
		NoRoute: true,
		Err:     err,
	}
}

// newExhaustedError builds the terminal error for a request whose candidate
// channels were all attempted and failed.
func newExhaustedError(ctx context.Context, modelName string, attempted []string, err error) error {
	return &DispatchError{
		Model: modelName,
		Trail: formatAttemptedTrail(ctx, attempted),
		Err:   err,
	}
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
		cause = ctx.Err()
	}
	return &DispatchError{
		Trail:        formatChannelTrail(ctx, attempted, channel),
		PromptTokens: promptTokens,
		Err:          cause,
	}
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
