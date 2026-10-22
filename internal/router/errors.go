package router

import (
	"context"
	"errors"
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
