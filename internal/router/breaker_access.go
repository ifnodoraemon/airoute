package router

// =============================================================================
// Circuit-breaker accessors for auxiliary upstream calls
//
// Most upstream traffic goes through Dispatch / DispatchStream / DispatchHTTP /
// DispatchEmbedding / DispatchRerank, which own the breaker bookkeeping. A few
// data-plane helpers dial a channel directly instead, because they do not fit the
// dispatch contract: /v1/messages/count_tokens is an Anthropic-wire-only auxiliary
// endpoint (it is not billed, has no model fallback and no first-token window), and
// admin probes are deliberately out-of-band. Those callers used to bypass the
// breaker entirely (B-07): they hammered OPEN channels and never reported the
// outcome back, so a channel could only be healed or tripped by real traffic.
//
// These four methods are the minimal bridge for such callers. They are thin
// wrappers on purpose: all state, thresholds, canary accounting and advisory-mode
// behaviour stay in CircuitBreaker.
// =============================================================================

// ChannelCanExecute is the admission check for an auxiliary upstream call
// (count_tokens, out-of-band probes).
//
// It has the same mutating semantics as CircuitBreaker.CanExecute: in half-open
// state it consumes a canary slot, so a caller that then abandons the attempt
// without an outcome MUST call ReleaseHalfOpenProbe. With the breaker in advisory
// mode (GATEWAY_BREAKER_ENABLED=false) it always admits traffic.
func (d *Dispatcher) ChannelCanExecute(name string) bool {
	if d == nil || d.circuitBreaker == nil {
		return true
	}
	return d.circuitBreaker.CanExecute(name)
}

// NoteChannelSuccess reports a successful auxiliary upstream call: the channel
// delivered a usable answer, so the breaker resets its failure streak (or, in
// half-open state, consumes the canary as a success).
func (d *Dispatcher) NoteChannelSuccess(name string) {
	if d == nil || d.circuitBreaker == nil {
		return
	}
	d.circuitBreaker.RecordSuccess(name)
}

// NoteChannelFailure reports an unhealthy auxiliary upstream call, using the same
// threshold and cooldown as dispatch traffic. In advisory mode the failure is
// recorded for operators but never blocks traffic (CircuitBreaker.RecordFailure
// already implements that).
func (d *Dispatcher) NoteChannelFailure(name string) {
	if d == nil || d.circuitBreaker == nil {
		return
	}
	d.circuitBreaker.RecordFailure(name)
}

// ReleaseHalfOpenProbe hands a half-open canary slot back without claiming either
// outcome. Callers use it when the attempt ended for a reason that is not a
// channel-health signal: the client went away, the request payload could not be
// built locally, or the failure classification says "retry elsewhere, do not
// charge the breaker". Without it a rate-limited canary would keep the channel
// locked out of rotation until the 2x-cooldown stale-slot reset.
func (d *Dispatcher) ReleaseHalfOpenProbe(name string) {
	if d == nil || d.circuitBreaker == nil {
		return
	}
	d.circuitBreaker.AbortCanary(name)
}
