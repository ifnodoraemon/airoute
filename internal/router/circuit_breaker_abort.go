package router

// AbortCanary decrements the canary in-flight count if a canary probe was aborted
// (e.g. client cancellation) during half-open recovery, preventing canary lockouts.
func (cb *CircuitBreaker) AbortCanary(name string) {
	cb.abortCanary(name, nil)
}

// AbortCanaryGen releases only the slot acquired in the same breaker generation.
func (cb *CircuitBreaker) AbortCanaryGen(name string, gen uint64) {
	cb.abortCanary(name, &gen)
}

func (cb *CircuitBreaker) abortCanary(name string, gen *uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists || (gen != nil && status.Generation != *gen) {
		return
	}
	if status.State == StateHalfOpen && status.CanaryInFlight > 0 {
		status.CanaryInFlight--
	}
}
