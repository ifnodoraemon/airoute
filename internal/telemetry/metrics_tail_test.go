package telemetry

import (
	"testing"
	"time"
)

func newLatencyTestMetrics() *Metrics {
	return &Metrics{
		latencyBuckets: make(map[float64]uint64),
		ttftBuckets:    make(map[float64]uint64),
		modelRequests:  make(map[string]map[string]uint64),
	}
}

// A long stream used to fall outside the histogram (top bucket was 60s), so p99
// was reported as exactly 60000 no matter how slow the real tail was.
func TestLatencyP99ReportsBeyondTheLastBucket(t *testing.T) {
	m := newLatencyTestMetrics()
	// 190 fast requests plus 10 very slow ones: the p99 rank then falls inside
	// the tail, which is exactly the case the old 60s ceiling could not express.
	for i := 0; i < 190; i++ {
		m.RecordRequestWithModel("", true, 100*time.Millisecond, 0, 0)
	}
	for i := 0; i < 10; i++ {
		m.RecordRequestWithModel("", true, 113*time.Second, 0, 0)
	}

	_, _, p99 := m.GetLatencyPercentiles()
	if p99 <= 60000 {
		t.Fatalf("p99 = %.0fms: a 113s observation must not be clamped at 60s", p99)
	}
	if p99 > 120000 {
		t.Fatalf("p99 = %.0fms, want an interpolated value within the tail bucket", p99)
	}
}

// When every observation is slower than the widest bucket, the percentiles must
// report the observed maximum rather than the bucket ceiling.
func TestLatencyPercentilesFallBackToObservedMax(t *testing.T) {
	m := newLatencyTestMetrics()
	for i := 0; i < 10; i++ {
		m.RecordRequestWithModel("", true, 700*time.Second, 0, 0)
	}
	p50, p90, p99 := m.GetLatencyPercentiles()
	for _, got := range []float64{p50, p90, p99} {
		if got < 700000 {
			t.Fatalf("percentile = %.0fms, want the observed 700000ms maximum", got)
		}
	}
}

// TTFT has the same problem: the average is already above the old top bucket.
func TestTTFTMaxIsTracked(t *testing.T) {
	m := newLatencyTestMetrics()
	m.RecordTTFT(90 * time.Second)
	if m.maxTTFTMs < 90000 {
		t.Fatalf("maxTTFTMs = %.0f, want 90000 recorded", m.maxTTFTMs)
	}
	m.RecordTTFT(time.Millisecond)
	if m.maxTTFTMs < 90000 {
		t.Fatal("a fast sample lowered the observed maximum")
	}
}
