package provider

import (
	"testing"
)

// A rate limit is backpressure from a healthy upstream, not an outage. Treating it
// as a channel failure opens the circuit and replaces the actionable 429 with a
// gateway 502 for the whole cooldown window.
//
// This test pins the contract documented on IsChannelFailure (internal/provider/
// errors.go): 429 is retried on the next channel but never counted, while
// 401/403/408/5xx stay counted. 404 is not counted either and must not be treated
// as a fatal payload error (see TestPayloadClassificationKeeps404Retryable).
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
