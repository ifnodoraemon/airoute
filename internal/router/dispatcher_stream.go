package router

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// DispatchStream executes streaming chat with Safe Fallback Window before the first token.
func (d *Dispatcher) DispatchStream(ctx context.Context, req *model.ChatCompletionRequest) (<-chan *model.StreamEvent, error) {
	proto := "chat"
	if req.Protocol != "" {
		proto = req.Protocol
	}
	channels := d.ResolveCandidateChannels(ctx, req.Model, proto)
	if len(channels) == 0 {
		if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
			nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
			if fbErr != nil {
				telemetry.Logger.Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
				return nil, fmt.Errorf("no upstream provider available for requested model '%s': %w", req.Model, fbErr)
			}
			telemetry.Logger.Warn("no upstream provider for stream model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.DispatchStream(nextCtx, &reqCopy)
		}
		return nil, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model)
	}

	var lastErr error
	var attempted []string // channels actually called and failed for this model
	for i, ch := range channels {
		// High-Availability Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.Logger.Warn("circuit breaker is OPEN, bypassing dead upstream stream", "channel", ch.Name)
			continue
		}

		d.mu.RLock()
		prov, exists := d.providers[ch.Type]
		d.mu.RUnlock()

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			continue
		}

		telemetry.Logger.Info("attempting streaming connection",
			"channel", ch.Name,
			"model", req.Model,
			"attempt", i+1,
		)

		attemptCtx, attemptCancel := context.WithCancel(ctx)
		streamChan, err := prov.ChatCompleteStream(attemptCtx, req, ch)
		if err != nil {
			attemptCancel()
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = err
			attempted = append(attempted, ch.Name)
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("stream connection failed, trying next channel",
				"channel", ch.Name,
				"error", err.Error(),
			)
			continue
		}

		firstTokenTimeout := d.GetFirstTokenTimeout(ch, req.Model)
		select {
		case firstEvent, ok := <-streamChan:
			if !ok {
				attemptCancel()
				d.circuitBreaker.RecordFailure(ch.Name)
				lastErr = fmt.Errorf("channel %s closed stream without events", ch.Name)
				attempted = append(attempted, ch.Name)
				telemetry.GlobalMetrics.RecordFallback()
				continue
			}

			if firstEvent.Err != nil {
				attemptCancel()
				d.circuitBreaker.RecordFailure(ch.Name)
				lastErr = firstEvent.Err
				attempted = append(attempted, ch.Name)
				telemetry.GlobalMetrics.RecordFallback()
				telemetry.Logger.Warn("channel failed before first valid token, falling back",
					"channel", ch.Name,
					"error", firstEvent.Err.Error(),
				)
				continue
			}

			// First token healthy! Mark healthy in circuit breaker
			d.circuitBreaker.RecordSuccess(ch.Name)
			// Expose the full routing chain (e.g. "A→B") for audit logging.
			trail := formatChannelTrail(ctx, attempted, ch.Name)
			firstEvent.Channel = trail

			// Wrap and return combined stream with leak-proof cancellation context
			outChan := make(chan *model.StreamEvent, 64)
			go pipeStreamEvents(ctx, outChan, firstEvent, streamChan, trail, attemptCancel)

			return outChan, nil

		case <-time.After(firstTokenTimeout):
			attemptCancel()
			// Drain remaining events in background so upstream sender unblocks
			go func(c <-chan *model.StreamEvent) {
				for range c {
				}
			}(streamChan)
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = fmt.Errorf("channel %s timed out waiting for first token (%v)", ch.Name, firstTokenTimeout)
			attempted = append(attempted, ch.Name)
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("first token timeout, falling back", "channel", ch.Name, "timeout", firstTokenTimeout)
			continue

		case <-ctx.Done():
			attemptCancel()
			return nil, ctx.Err()
		}
	}

	if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
		nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
		if fbErr != nil {
			telemetry.Logger.Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
			return nil, fmt.Errorf("all channels failed for stream request on model %s (last error: %v); cannot fallback to %s: %w", req.Model, lastErr, fb, fbErr)
		}
		telemetry.Logger.Warn("all stream channels failed, triggering cross-model fallback",
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.DispatchStream(withChannelTrail(nextCtx, attempted), &reqCopy)
	}

	return nil, fmt.Errorf("all channels failed for stream request on model %s. Last error: %w", req.Model, lastErr)
}

// pipeStreamEvents pipes events from upstream stream channel to client channel with cancellation guard.
func pipeStreamEvents(ctx context.Context, outChan chan<- *model.StreamEvent, first *model.StreamEvent, in <-chan *model.StreamEvent, trail string, cancel context.CancelFunc) {
	defer cancel()
	defer close(outChan)
	select {
	case outChan <- first:
	case <-ctx.Done():
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-in:
			if !ok {
				return
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

// GetFirstTokenTimeout determines the safe fallback window timeout for a channel and model.
// Deep thinking / reasoning models (e.g. o1, o3, DeepSeek-R1) are given an extended window (up to 60s),
// and the environment variable GATEWAY_FIRST_TOKEN_TIMEOUT can explicitly override the default 30s.
func (d *Dispatcher) GetFirstTokenTimeout(ch *model.ChannelConfig, modelName string) time.Duration {
	timeout := 30 * time.Second
	lowerModel := strings.ToLower(modelName)
	if strings.Contains(lowerModel, "r1") || strings.Contains(lowerModel, "o1") || strings.Contains(lowerModel, "o3") || strings.Contains(lowerModel, "reasoning") || strings.Contains(lowerModel, "thinking") {
		timeout = 60 * time.Second
	}
	if env := os.Getenv("GATEWAY_FIRST_TOKEN_TIMEOUT"); env != "" {
		if s, err := strconv.Atoi(env); err == nil && s > 0 {
			timeout = time.Duration(s) * time.Second
		}
	}
	if ch != nil && ch.TimeoutSeconds > 0 {
		chTimeout := time.Duration(ch.TimeoutSeconds) * time.Second
		if timeout > chTimeout {
			timeout = chTimeout
		}
	}
	return timeout
}
