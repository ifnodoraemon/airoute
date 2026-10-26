package router

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

type fallbackChainKey struct{}

func getFallbackChain(ctx context.Context) []string {
	if val := ctx.Value(fallbackChainKey{}); val != nil {
		if chain, ok := val.([]string); ok {
			return chain
		}
	}
	return nil
}

func nextFallbackContext(ctx context.Context, currentModel, nextModel string) (context.Context, error) {
	const maxFallbackDepth = 4
	chain := getFallbackChain(ctx)
	if len(chain) == 0 {
		chain = []string{currentModel}
	}
	if len(chain) >= maxFallbackDepth {
		return ctx, fmt.Errorf("cross-model fallback limit (%d) reached (chain: %s -> %s)", maxFallbackDepth, strings.Join(chain, " -> "), nextModel)
	}
	for _, m := range chain {
		if strings.EqualFold(m, nextModel) {
			return ctx, fmt.Errorf("cross-model fallback cycle detected (%s -> %s)", strings.Join(chain, " -> "), nextModel)
		}
	}
	newChain := append(append([]string(nil), chain...), nextModel)
	return context.WithValue(ctx, fallbackChainKey{}, newChain), nil
}

// channelTrailKey carries the sequence of channels that were actually called
// (and failed) across cross-model fallback recursions, so the final audit log
// can show the full routing chain such as "A→B→C".
type channelTrailKey struct{}

func getChannelTrail(ctx context.Context) []string {
	if val := ctx.Value(channelTrailKey{}); val != nil {
		if trail, ok := val.([]string); ok {
			return trail
		}
	}
	return nil
}

// withChannelTrail returns a context carrying prior failures plus the given
// attempted channels, for propagation into fallback recursion.
func withChannelTrail(ctx context.Context, attempted []string) context.Context {
	if len(attempted) == 0 {
		return ctx
	}
	merged := append(append([]string(nil), getChannelTrail(ctx)...), attempted...)
	return context.WithValue(ctx, channelTrailKey{}, merged)
}

// maxChannelTrailLen caps the rendered trail: the usage_logs.channel column is
// VARCHAR(128) on Postgres.
const maxChannelTrailLen = 128

// formatChannelTrail renders the full routing chain for the audit log:
// channels attempted in prior models (from ctx) + channels attempted for the
// current model + the channel that finally succeeded, joined with "→".
func formatChannelTrail(ctx context.Context, attempted []string, final string) string {
	prior := getChannelTrail(ctx)
	all := make([]string, 0, len(prior)+len(attempted)+1)
	all = append(all, prior...)
	all = append(all, attempted...)
	all = append(all, final)
	return joinChannelTrail(all)
}

// formatAttemptedTrail renders the routing chain that was actually called and
// failed, with no serving channel appended. This is the failure-path counterpart
// of formatChannelTrail, used by DispatchError: it keeps the audit trail honest
// when there is no success hop, and returns "" when no channel was ever reached.
func formatAttemptedTrail(ctx context.Context, attempted []string) string {
	prior := getChannelTrail(ctx)
	if len(prior) == 0 && len(attempted) == 0 {
		return ""
	}
	all := make([]string, 0, len(prior)+len(attempted))
	all = append(all, prior...)
	all = append(all, attempted...)
	return joinChannelTrail(all)
}

// joinChannelTrail joins a routing chain with "→", capped to maxChannelTrailLen
// runes; when capped, the most recent hops are preserved and the chain is
// prefixed with "…".
func joinChannelTrail(all []string) string {
	joined := strings.Join(all, "→")
	if len([]rune(joined)) <= maxChannelTrailLen {
		return joined
	}
	// Keep the most recent hops so the serving channel always survives.
	out := ""
	for i := len(all) - 1; i >= 0; i-- {
		candidate := all[i]
		if out != "" {
			candidate += "→" + out
		}
		if len([]rune(candidate))+1 > maxChannelTrailLen { // +1 rune for the "…" prefix
			break
		}
		out = candidate
	}
	if out == "" {
		// The final hop alone exceeds the cap: keep its tail so the serving
		// channel always survives in the audit trail (never a bare "…").
		r := []rune(all[len(all)-1])
		if keep := maxChannelTrailLen - 1; len(r) > keep {
			r = r[len(r)-keep:]
		}
		return "…" + string(r)
	}
	return "…" + out
}

// recordChannelFailure records circuit breaker status, telemetry fallback counter, and structured logs.
func (d *Dispatcher) recordChannelFailure(ctx context.Context, chName string, err error, attempt int) string {
	d.circuitBreaker.RecordFailure(chName)
	telemetry.GlobalMetrics.RecordFallback()
	routerLog(ctx).Warn("channel execution failed, triggering fallback",
		"channel", chName,
		"error", err.Error(),
		"attempt", attempt,
	)
	return chName
}

// recordChannelSuccess records circuit breaker status, telemetry request metrics, and returns the full audit trail.
func (d *Dispatcher) recordChannelSuccess(ctx context.Context, attempted []string, chName string, dur time.Duration, promptTokens, compTokens int) string {
	d.circuitBreaker.RecordSuccess(chName)
	trail := formatChannelTrail(ctx, attempted, chName)
	telemetry.GlobalMetrics.RecordRequest(true, dur, promptTokens, compTokens)
	routerLog(ctx).Debug("channel execution succeeded",
		"channel", chName,
		"duration_ms", dur.Milliseconds(),
		"total_tokens", promptTokens+compTokens,
	)
	return trail
}
