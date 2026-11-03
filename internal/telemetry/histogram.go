package telemetry

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
)

// Histogram is a fixed-bucket, lock-free histogram for hot-path durations.
//
// The metric paths in this gateway are scraped rarely but written per request,
// so a sync.RWMutex-protected map (the shape the latency percentiles use) is the
// wrong trade-off here: settlement happens on the request path and must never
// queue behind a scrape. Each observation is a handful of atomic adds.
//
// Sums are accumulated as integer microseconds to stay lock-free; observations
// are durations and batch sizes, both far below the 1e6-µs-per-unit granularity
// that would make a single observation overflow the counter.
type Histogram struct {
	bounds []float64
	// counts has len(bounds) finite buckets plus one +Inf bucket.
	counts []atomic.Uint64
	sumUS  atomic.Uint64
	count  atomic.Uint64
}

// NewHistogram builds a histogram with the given upper bounds, which must be
// strictly increasing. The upper bounds are the values reported as `le`.
func NewHistogram(bounds []float64) *Histogram {
	sorted := make([]float64, len(bounds))
	copy(sorted, bounds)
	h := &Histogram{
		bounds: sorted,
		counts: make([]atomic.Uint64, len(sorted)+1),
	}
	return h
}

// RecordObserved adds one observation.
func (h *Histogram) Observe(value float64) {
	if h == nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	bucket := len(h.bounds)
	for i, bound := range h.bounds {
		if value <= bound {
			bucket = i
			break
		}
	}
	h.counts[bucket].Add(1)
	// Microsecond scaling keeps the sum integral without a CAS loop. Doubles
	// (durations in seconds, batch sizes) are well below the overflow point.
	if value > 0 {
		h.sumUS.Add(uint64(math.Round(value * 1e6)))
	} else if value < 0 {
		h.sumUS.Add(0)
	}
	h.count.Add(1)
}

// Snapshot returns cumulative bucket counts (matching Prometheus semantics,
// including the +Inf bucket), the sum, and the total observation count.
func (h *Histogram) Snapshot() (buckets []uint64, sum float64, total uint64) {
	if h == nil {
		return nil, 0, 0
	}
	buckets = make([]uint64, len(h.counts))
	var cumulative uint64
	for i := range h.counts {
		cumulative += h.counts[i].Load()
		buckets[i] = cumulative
	}
	return buckets, float64(h.sumUS.Load()) / 1e6, h.count.Load()
}

// WritePrometheus renders the histogram in the Prometheus text exposition
// format. bucketValues must be the result of Snapshot so the caller exports a
// consistent point-in-time view.
func (h *Histogram) WritePrometheus(sb *strings.Builder, name, help string) {
	values, sum, total := h.Snapshot()
	if values == nil {
		return
	}
	fmt.Fprintf(sb, "# HELP %s %s\n", name, help)
	fmt.Fprintf(sb, "# TYPE %s histogram\n", name)
	for i := range h.bounds {
		fmt.Fprintf(sb, "%s_bucket{le=%q} %d\n", name, formatBucket(h.bounds[i]), values[i])
	}
	fmt.Fprintf(sb, "%s_bucket{le=\"+Inf\"} %d\n", name, values[len(values)-1])
	fmt.Fprintf(sb, "%s_sum %s\n", name, formatFloat(sum))
	fmt.Fprintf(sb, "%s_count %d\n", name, total)
}

// formatBucket renders a bucket upper bound the way Prometheus expects: no
// scientific notation and no trailing zeros.
func formatBucket(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Prometheus-facing histograms. They are package-level rather than fields on
// Metrics so a zero-valued Metrics constructed in a test can never nil-deref
// during a scrape, and so call sites stay short.
var (
	// SettlementDurationSeconds measures one settlement transaction (the
	// database work itself), not the request-side wait.
	SettlementDurationSeconds = NewHistogram([]float64{
		0.0005, 0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5,
	})
	// SettlementBatchSize measures how many charges a group-commit flush merged.
	SettlementBatchSize = NewHistogram([]float64{
		1, 2, 5, 10, 25, 50, 100, 250, 500, 1000,
	})
)
