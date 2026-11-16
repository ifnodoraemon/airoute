package telemetry

// Latency percentile estimation, extracted from metrics.go to keep the core
// counters file under the size red line.

// GetLatencyPercentiles estimates P50, P90, and P99 latency percentiles in milliseconds.
func (m *Metrics) GetLatencyPercentiles() (p50, p90, p99 float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.durCount == 0 {
		return 0, 0, 0
	}
	p50 = estimateQuantile(0.50, m.durCount, defaultLatencyBuckets, m.latencyBuckets, m.maxDurMs)
	p90 = estimateQuantile(0.90, m.durCount, defaultLatencyBuckets, m.latencyBuckets, m.maxDurMs)
	p99 = estimateQuantile(0.99, m.durCount, defaultLatencyBuckets, m.latencyBuckets, m.maxDurMs)
	return p50, p90, p99
}

// estimateQuantile interpolates within cumulative "less or equal" buckets.
// overflowMax is the largest observation actually recorded; it is reported when
// the target rank lies beyond the widest bucket, so a tail slower than the last
// bucket is never silently clamped to that bucket's upper bound.
func estimateQuantile(q float64, totalCount uint64, buckets []float64, counts map[float64]uint64, overflowMax float64) float64 {
	targetRank := q * float64(totalCount)
	var prevLe float64 = 0
	var prevCount uint64 = 0

	for _, b := range buckets {
		cnt := counts[b]
		if float64(cnt) >= targetRank {
			bucketWidth := b - prevLe
			countInBucket := float64(cnt - prevCount)
			if countInBucket <= 0 {
				return b
			}
			rankInBucket := targetRank - float64(prevCount)
			return prevLe + (rankInBucket/countInBucket)*bucketWidth
		}
		prevLe = b
		prevCount = cnt
	}
	if len(buckets) > 0 {
		if overflowMax > buckets[len(buckets)-1] {
			return overflowMax
		}
		return buckets[len(buckets)-1]
	}
	return 0
}