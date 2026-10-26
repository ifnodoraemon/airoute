package provider

import (
	"testing"
)

// A rate limit is backpressure from a healthy upstream, not an outage. Treating it
// as a channel failure opens the circuit and replaces the actionable 429 with a
// gateway 502 for the whole cooldown window.
func TestRateLimitIsNotAChannelFailure(t *testing.T) {
	if IsChannelFailure(NewUpstreamError("ch", 429, []byte("slow down"))) {
		t.Fatal("429 must not count as a channel failure")
	}
	// 408 still signals an upstream-side problem and keeps tripping the breaker.
	for _, status := range []int{401, 403, 408, 500, 502, 503} {
		if !IsChannelFailure(NewUpstreamError("ch", status, []byte("boom"))) {
			t.Fatalf("%d should still count as a channel failure", status)
		}
	}
}
