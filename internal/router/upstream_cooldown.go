package router

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// admissionError is local backpressure: no upstream call was made, and the
// error must neither trip the breaker nor invalidate a working session pin.
type admissionError struct {
	reason string
	until  time.Time
}

func (e *admissionError) Error() string { return e.reason }
func (e *admissionError) Unwrap() error {
	seconds := max(1, int(time.Until(e.until).Seconds()+1))
	return provider.NewUpstreamError("", http.StatusTooManyRequests, []byte(e.reason), http.Header{"Retry-After": {strconv.Itoa(seconds)}})
}

type cooldowns struct {
	mu          sync.Mutex
	until       map[string]time.Time
	defaultWait time.Duration
}

func newCooldowns() *cooldowns {
	wait := time.Second
	if n, err := strconv.Atoi(os.Getenv("GATEWAY_UPSTREAM_COOLDOWN_SEC")); err == nil && n >= 0 && n <= 86400 {
		wait = time.Duration(n) * time.Second
	}
	return &cooldowns{until: make(map[string]time.Time), defaultWait: wait}
}

// upstreamIdentity shares protection across duplicate channels using the same
// endpoint and credential. Only a digest is retained in scheduling state.
func upstreamIdentity(ch *model.ChannelConfig) string { return model.UpstreamIdentity(ch) }

func cooldownKey(ch *model.ChannelConfig, modelName string) string {
	return upstreamIdentity(ch) + "\x00" + ch.GetUpstreamModel(modelName)
}

func (c *cooldowns) check(ch *model.ChannelConfig, modelName string) error {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	until := maxTime(c.until[cooldownKey(ch, modelName)], c.until[upstreamIdentity(ch)+"\x00*"])
	if now.Before(until) {
		return &admissionError{reason: "upstream is cooling down after a rate limit; no upstream was called", until: until}
	}
	return nil
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (c *cooldowns) record(ch *model.ChannelConfig, modelName string, err error) {
	var local *admissionError
	if errors.As(err, &local) {
		return
	}
	ue, ok := provider.AsUpstreamError(err)
	if !ok || ue.Status != http.StatusTooManyRequests {
		return
	}
	now := time.Now()
	wait := max(c.defaultWait, provider.RetryDelay(ue.Headers, now))
	if wait <= 0 {
		return
	}
	key := cooldownKey(ch, modelName)
	switch strings.ToLower(ue.Headers.Get("X-RateLimit-Scope")) {
	case "account", "key", "global":
		key = upstreamIdentity(ch) + "\x00*"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.until) >= maxSessionPins {
		var oldest string
		var first time.Time
		for k, until := range c.until {
			if !now.Before(until) {
				delete(c.until, k)
			} else if first.IsZero() || until.Before(first) {
				oldest, first = k, until
			}
		}
		if len(c.until) >= maxSessionPins {
			delete(c.until, oldest)
		}
	}
	c.until[key] = maxTime(c.until[key], now.Add(wait))
}

func (d *Dispatcher) observeUpstreamError(ctx context.Context, ch *model.ChannelConfig, modelName string, err error) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	d.cooldowns.record(ch, modelName, err)
	d.forgetSessionChannel(ctx, modelName, ch.Name)
}

func (d *Dispatcher) beginChannelCall(ctx context.Context, ch *model.ChannelConfig, modelName string) (context.Context, context.CancelFunc, error) {
	if err := d.cooldowns.check(ch, modelName); err != nil {
		return nil, nil, err
	}
	release, err := d.lanes.acquire(ctx, ch)
	if err != nil {
		return nil, nil, err
	}
	if err := d.cooldowns.check(ch, modelName); err != nil {
		release()
		return nil, nil, err
	}
	callCtx, cancel := channelCallContext(ctx, ch)
	return callCtx, sync.OnceFunc(func() { cancel(); release() }), nil
}

func admissionDispatchError(ctx context.Context, modelName string, attempted []string, err error) error {
	if requestBudgetExceeded(ctx, ctx.Err()) {
		return newRequestBudgetError(modelName, attempted, err)
	}
	return newCanceledError(ctx, attempted, "", 0)
}
