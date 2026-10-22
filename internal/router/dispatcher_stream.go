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

// DispatchStream executes streaming chat with Safe Fallback Window before the first token.
func (d *Dispatcher) DispatchStream(ctx context.Context, req *model.ChatCompletionRequest) (<-chan *model.StreamEvent, error) {
	proto := "chat"
	if req.Protocol != "" {
		proto = req.Protocol
	}
	channels := d.candidateChannelsForRequest(ctx, req, proto)
	if len(channels) == 0 {
		if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
			nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
			if fbErr != nil {
				telemetry.ScopedLogger("router").Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
				return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for requested model '%s': %w", req.Model, fbErr))
			}
			telemetry.ScopedLogger("router").Warn("no upstream provider for stream model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.DispatchStream(nextCtx, &reqCopy)
		}
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed for this model
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// High-Availability Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.ScopedLogger("router").Warn("circuit breaker is OPEN, bypassing dead upstream stream", "channel", ch.Name)
			skipped = append(skipped, ch.Name)
			continue
		}

		prov, exists := d.getProviderForChannel(ch, req)

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		telemetry.ScopedLogger("router").Info("attempting streaming connection",
			"channel", ch.Name,
			"model", req.Model,
			"attempt", i+1,
		)

		attemptCtx, attemptCancel := context.WithCancel(ctx)
		streamChan, err := prov.ChatCompleteStream(attemptCtx, req, ch)
		if err != nil {
			attemptCancel()
			if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, ctx.Err()
			}
			// Client syntax/schema errors (400, 422, 413) fail identically on every channel:
			// do not trip the breaker and do not fan out.
			// Node-specific 404 (model missing on this instance) falls through to next candidate channel.
			if isFatalClientPayloadError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if provider.IsChannelFailure(err) {
				d.circuitBreaker.RecordFailure(ch.Name)
			} else {
				d.circuitBreaker.AbortCanary(ch.Name)
			}
			lastErr = err
			attempted = append(attempted, ch.Name)
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.ScopedLogger("router").Warn("stream connection failed, trying next channel",
				"channel", ch.Name,
				"error", err.Error(),
			)
			continue
		}

		firstTokenTimeout := d.GetFirstTokenTimeout(ch, req.Model)
		timer := time.NewTimer(firstTokenTimeout)
		var preamble []*model.StreamEvent
		var firstErr error
		committed := false
		for !committed && firstErr == nil {
			select {
			case event, ok := <-streamChan:
				if !ok {
					firstErr = fmt.Errorf("channel %s closed stream before a valid token", ch.Name)
					break
				}
				if event == nil {
					continue
				}
				if event.Err != nil {
					firstErr = event.Err
					break
				}
				// Role-only chunks and unparsed SSE frames are not evidence of a usable stream.
				// Keep a bounded preamble so it can be sent if content follows.
				if len(preamble) < 64 {
					preamble = append(preamble, event)
				}
				if streamEventHasContent(event) || event.IsDone {
					committed = true
				}
			case <-timer.C:
				firstErr = fmt.Errorf("channel %s timed out waiting for first token (%v)", ch.Name, firstTokenTimeout)
			case <-ctx.Done():
				timer.Stop()
				attemptCancel()
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, ctx.Err()
			}
		}
		timer.Stop()
		if firstErr != nil {
			attemptCancel()
			if ctx.Err() == context.Canceled || errors.Is(firstErr, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, ctx.Err()
			}
			if isFatalClientPayloadError(firstErr) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, firstErr
			}
			if provider.IsChannelFailure(firstErr) {
				d.circuitBreaker.RecordFailure(ch.Name)
			} else {
				d.circuitBreaker.AbortCanary(ch.Name)
			}
			lastErr = firstErr
			attempted = append(attempted, ch.Name)
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.ScopedLogger("router").Warn("channel failed before first valid token, falling back", "channel", ch.Name, "error", firstErr.Error())
			continue
		}

		d.circuitBreaker.RecordSuccess(ch.Name)
		trail := formatChannelTrail(ctx, attempted, ch.Name)
		outChan := make(chan *model.StreamEvent, 64)
		go pipeStreamEvents(ctx, outChan, preamble, streamChan, trail, attemptCancel)
		return outChan, nil
	}

	// Only skipped channels (all circuit-broken) leave lastErr nil; name that
	// condition instead of letting %w render "%!w(<nil>)" in the audit log.
	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}

	if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
		nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
		if fbErr != nil {
			telemetry.ScopedLogger("router").Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
			return nil, newExhaustedError(ctx, req.Model, attempted, fmt.Errorf("all channels failed for stream request on model %s (last error: %v); cannot fallback to %s: %w", req.Model, lastErr, fb, fbErr))
		}
		telemetry.ScopedLogger("router").Warn("all stream channels failed, triggering cross-model fallback",
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.DispatchStream(withChannelTrail(nextCtx, attempted), &reqCopy)
	}

	return nil, newExhaustedError(ctx, req.Model, attempted, fmt.Errorf("all channels failed for stream request on model %s. Last error: %w", req.Model, lastErr))
}

// pipeStreamEvents pipes events from upstream stream channel to client channel with cancellation guard.
func pipeStreamEvents(ctx context.Context, outChan chan<- *model.StreamEvent, preamble []*model.StreamEvent, in <-chan *model.StreamEvent, trail string, cancel context.CancelFunc) {
	defer cancel()
	defer close(outChan)
	for _, event := range preamble {
		event.Channel = trail
		select {
		case outChan <- event:
		case <-ctx.Done():
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-in:
			if !ok {
				return
			}
			if event == nil {
				continue
			}
			event.Channel = trail
			select {
			case outChan <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

func streamEventHasContent(event *model.StreamEvent) bool {
	if event == nil || event.Chunk == nil {
		return event != nil && (event.NativeAnthropicType == "content_block_delta" || event.NativeAnthropicType == "content_block_start")
	}
	for _, choice := range event.Chunk.Choices {
		if choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" || len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

// GetFirstTokenTimeout determines the safe fallback window timeout for a channel and model.
// Deep thinking / reasoning models (e.g. o1, o3, DeepSeek-R1) are given an extended window (up to 60s),
// and the environment variable GATEWAY_FIRST_TOKEN_TIMEOUT can explicitly override the default 30s.
// firstTokenTimeoutOverride reads GATEWAY_FIRST_TOKEN_TIMEOUT once per process.
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
	timeout := 30 * time.Second
	lowerModel := strings.ToLower(modelName)
	if strings.Contains(lowerModel, "r1") || strings.Contains(lowerModel, "o1") || strings.Contains(lowerModel, "o3") || strings.Contains(lowerModel, "reasoning") || strings.Contains(lowerModel, "thinking") {
		timeout = 60 * time.Second
	}
	if override := firstTokenTimeoutOverride(); override > 0 {
		timeout = override
	}
	if ch != nil && ch.TimeoutSeconds > 0 {
		chTimeout := time.Duration(ch.TimeoutSeconds) * time.Second
		if timeout > chTimeout {
			timeout = chTimeout
		}
	}
	return timeout
}
