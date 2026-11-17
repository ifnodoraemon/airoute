package router

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// failStreamAttempt classifies one failed stream attempt. It updates the circuit
// breaker and the fallback counter, and returns a non-nil error when the dispatch
// must stop instead of fanning out to the next channel: a client cancellation, or
// a client payload error that would fail identically everywhere. A node-specific
// 404 falls through to the next candidate channel, like any other channel failure.
func (d *Dispatcher) failStreamAttempt(ctx context.Context, req *model.ChatCompletionRequest, ch *model.ChannelConfig, attempted, skipped []string, err error, logMsg string, breakerGen uint64) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		d.circuitBreaker.AbortCanaryGen(ch.Name, breakerGen)
		return stoppedDispatchError(ctx, req.Model, attempted, skipped, ch.Name, req.EstimatedPromptTokens(), err)
	}
	d.observeUpstreamError(ctx, ch, req.Model, err)
	if provider.IsModelNotFound(err) {
		// A node-specific 404 is not the caller's fault: keep fanning out.
		d.circuitBreaker.AbortCanaryGen(ch.Name, breakerGen)
	} else if provider.IsFatalPayloadError(err) {
		d.circuitBreaker.AbortCanaryGen(ch.Name, breakerGen)
		return newDispatchError(ctx, req.Model, append(attempted, ch.Name), skipped, err)
	} else if provider.IsChannelFailure(err) {
		d.circuitBreaker.RecordFailureGen(ch.Name, breakerGen)
	} else {
		d.circuitBreaker.AbortCanaryGen(ch.Name, breakerGen)
	}
	telemetry.GlobalMetrics.RecordFallback()
	routerLog(ctx).Warn(logMsg, "channel", ch.Name, "error", err.Error())
	return nil
}

// DispatchStream executes streaming chat with Safe Fallback Window before the first token.
func (d *Dispatcher) DispatchStream(ctx context.Context, req *model.ChatCompletionRequest) (<-chan *model.StreamEvent, error) {
	proto := "chat"
	if req.Protocol != "" {
		proto = req.Protocol
	}
	var intentErr error
	req, intentErr = d.withThinkingIntent(req)
	if intentErr != nil {
		return nil, intentErr
	}
	channels, capabilityErr := d.candidateChannelsForRequest(ctx, req, proto)
	if len(channels) == 0 {
		if fb := d.requestFallback(req); fb != "" && !strings.EqualFold(fb, req.Model) {
			nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
			if fbErr != nil {
				routerLog(ctx).Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
				return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for requested model '%s': %w", req.Model, fbErr))
			}
			routerLog(ctx).Warn("no upstream provider for stream model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.DispatchStream(nextCtx, &reqCopy)
		}
		if capabilityErr != nil {
			return nil, capabilityErr
		}
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed for this model
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// High-Availability Circuit Breaker: fail fast if upstream is down
		callReq, thinkingErr := provider.PrepareThinkingRequest(req, ch)
		if thinkingErr != nil {
			lastErr = thinkingErr
			continue
		}

		admitted, breakerGen := d.circuitBreaker.CanExecuteGen(ch.Name)
		if !admitted {
			telemetry.WarnLogger(routerLog(ctx), "breaker-open-stream:"+ch.Name, 30*time.Second,
				"circuit breaker is OPEN, bypassing dead upstream stream",
				"channel", ch.Name, "attempt", i+1)
			skipped = append(skipped, ch.Name)
			continue
		}

		prov, exists := d.getProviderForRequest(ch, callReq)

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			d.circuitBreaker.AbortCanaryGen(ch.Name, breakerGen)
			continue
		}

		routerLog(ctx).Debug("attempting streaming connection",
			"channel", ch.Name,
			"model", req.Model,
			"attempt", i+1,
		)

		// The provider call is bounded by the channel's timeout_seconds, exactly
		// like the non-streaming path: the gateway no longer imposes an absolute
		// drain deadline on the whole request, so a channel configured for e.g.
		// 300s is allowed to stream for 300s. The first-token window below is
		// derived from the same number.
		attemptCtx, attemptCancel, admissionErr := d.beginChannelCall(ctx, ch, req.Model)
		if admissionErr != nil {
			d.circuitBreaker.AbortCanaryGen(ch.Name, breakerGen)
			skipped = append(skipped, ch.Name)
			if ctx.Err() != nil {
				return nil, admissionDispatchError(ctx, req.Model, attempted, skipped, admissionErr)
			}
			lastErr = admissionErr
			logAdmissionFailure(ctx, ch.Name, attempted, skipped, admissionErr)
			continue
		}
		streamChan, err := prov.ChatCompleteStream(attemptCtx, callReq, ch)
		if err != nil {
			attemptCancel()
			if stopErr := d.failStreamAttempt(ctx, req, ch, attempted, skipped, err, "stream connection failed, trying next channel", breakerGen); stopErr != nil {
				return nil, stopErr
			}
			lastErr = err
			attempted = append(attempted, ch.Name)
			continue
		}

		preamble, firstErr := waitStreamPreamble(ctx, attemptCtx, streamChan, ch.Name, d.GetFirstTokenTimeout(ch, req.Model))
		if firstErr != nil {
			attemptCancel()
			if stopErr := d.failStreamAttempt(ctx, req, ch, attempted, skipped, firstErr, "channel failed before first valid token, falling back", breakerGen); stopErr != nil {
				return nil, stopErr
			}
			lastErr = firstErr
			attempted = append(attempted, ch.Name)
			continue
		}

		d.circuitBreaker.RecordSuccessGen(ch.Name, breakerGen)
		d.rememberSessionChannel(ctx, req.Model, ch.Name)
		trail := formatChannelTrail(ctx, attempted, ch.Name)
		outChan := make(chan *model.StreamEvent, 64)
		// Committing the stream only proves the channel produced a first token.
		// Anything that goes wrong after this point (mid-stream error frame,
		// truncated SSE, upstream-side timeout) is still a channel health signal,
		// so pipeStreamEvents reports it back here (B-02).
		recordPostCommitFailure := func(err error) {
			if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
				d.observeUpstreamError(ctx, ch, req.Model, err)
			}
			d.recordStreamFailure(ctx, ch.Name, err, i+1)
		}
		trace := beginThinkingTrace(ctx, callReq, ch)
		go pipeStreamEvents(ctx, outChan, preamble, streamChan, trail, attemptCancel, recordPostCommitFailure, trace.ObserveEvent)
		return outChan, nil
	}

	// Only skipped channels (all circuit-broken) leave lastErr nil; name that
	// condition instead of letting %w render "%!w(<nil>)" in the audit log.
	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}

	if fb := d.requestFallback(req); fb != "" && !strings.EqualFold(fb, req.Model) {
		nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
		if fbErr != nil {
			routerLog(ctx).Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
			return nil, newExhaustedError(ctx, req.Model, attempted, skipped, fmt.Errorf("all channels failed for stream request on model %s (last error: %v); cannot fallback to %s: %w", req.Model, lastErr, fb, fbErr))
		}
		routerLog(ctx).Warn("all stream channels failed, triggering cross-model fallback",
			"attempted", attempted, "skipped", skipped,
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.DispatchStream(withChannelTrail(nextCtx, attempted, skipped), &reqCopy)
	}

	return nil, newExhaustedError(ctx, req.Model, attempted, skipped, fmt.Errorf("all channels failed for stream request on model %s. Last error: %w", req.Model, lastErr))
}

// GetFirstTokenTimeout is how long a single attempt may stay silent before the
// gateway gives up on it. It is derived from the channel timeout, not configured
// separately.
// Deep thinking / reasoning models (e.g. o1, o3, DeepSeek-R1) are given an extended window (up to 60s),
// and the environment variable GATEWAY_FIRST_TOKEN_TIMEOUT can explicitly override the default 30s.
// firstTokenTimeoutOverride reads GATEWAY_FIRST_TOKEN_TIMEOUT once per process. It
// only applies to channels that have no timeout configured; a configured channel
// timeout is always the window that governs.
// It used to be parsed on every streaming request (os.Getenv + strconv.Atoi).
var firstTokenTimeoutOverride = sync.OnceValue(func() time.Duration {
	env := os.Getenv("GATEWAY_FIRST_TOKEN_TIMEOUT")
	if env == "" {
		return 0
	}
	s, err := strconv.Atoi(env)
	if err != nil || s <= 0 {
		return 0
	}
	return time.Duration(s) * time.Second
})

func (d *Dispatcher) GetFirstTokenTimeout(ch *model.ChannelConfig, modelName string) time.Duration {
	return resolveFirstTokenTimeout(ch, modelName, firstTokenTimeoutOverride())
}

// resolveFirstTokenTimeout derives the first-token window. The channel timeout
// is the budget an operator configures, so it is also the window a channel may
// stay silent; the reasoning-model default only applies when the channel sets no
// timeout. GATEWAY_FIRST_TOKEN_TIMEOUT is documented as an upper bound, so it can
// shorten that window but never extend it past the channel budget.
func resolveFirstTokenTimeout(ch *model.ChannelConfig, modelName string, override time.Duration) time.Duration {
	timeout := 30 * time.Second
	if ch != nil && ch.TimeoutSeconds > 0 {
		timeout = time.Duration(ch.TimeoutSeconds) * time.Second
	} else {
		lowerModel := strings.ToLower(modelName)
		if strings.Contains(lowerModel, "r1") || strings.Contains(lowerModel, "o1") || strings.Contains(lowerModel, "o3") || strings.Contains(lowerModel, "reasoning") || strings.Contains(lowerModel, "thinking") {
			timeout = 60 * time.Second
		}
	}
	if override > 0 && override < timeout {
		return override
	}
	return timeout
}
