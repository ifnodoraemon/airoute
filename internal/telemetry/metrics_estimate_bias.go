package telemetry

// Per-model estimator bias tracking, extracted from metrics.go to keep the core
// counters file under the size red line.

type estimateBiasSample struct {
	ppm     int64
	samples uint64
}

// weight is this sample's weight in the fleet-wide mean: the number of upstream
// reports behind it, or 1 when the count is unknown (a zero-weight sample would be
// invisible in the mean).
func (s estimateBiasSample) weight() int64 {
	if s.samples == 0 {
		return 1
	}
	return int64(s.samples)
}

// SetEstimateBias publishes the estimator bias for one model.
//
// The per-model value is what an operator needs (a single model with a bad
// tokenizer guess), and the unlabelled total is its sample-weighted mean, so
// neither number is decided by whichever request happened to be observed last.
//
// Two properties the previous implementation lacked:
//
//   - the model key is bounded like modelRequests: the name comes from the
//     client's request, so without modelLabelCap the map (and the Prometheus label
//     set) grew without bound for the process lifetime;
//   - the weighted mean is maintained incrementally (subtract the previous value,
//     add the new one), so this call is O(1) instead of re-summing every model on
//     each upstream usage report.
func (m *Metrics) SetEstimateBias(model string, ppm int64, samples uint64) {
	if model == "" {
		// An empty label carries no information; the previous implementation also
		// stored nothing for it, which left the mean untouched.
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.estimateBiasByModel == nil {
		m.estimateBiasByModel = make(map[string]estimateBiasSample)
	}

	key := model
	if _, known := m.estimateBiasByModel[key]; !known && len(m.estimateBiasByModel) >= modelLabelCap {
		key = modelLabelOverflow
	}
	next := estimateBiasSample{ppm: ppm, samples: samples}
	if key == modelLabelOverflow {
		// Several models share this bucket. Keep a merged, sample-weighted view of
		// them (rather than letting whichever model was observed last overwrite the
		// bucket), so the bounded label still says "traffic outside the top models is
		// mis-estimated by about this much".
		if previous, ok := m.estimateBiasByModel[key]; ok {
			prevWeight, nextWeight := previous.weight(), next.weight()
			mergedSamples := previous.samples + samples
			mergedPPM := (previous.ppm*prevWeight + ppm*nextWeight) / (prevWeight + nextWeight)
			next = estimateBiasSample{ppm: mergedPPM, samples: mergedSamples}
		}
	}

	if previous, ok := m.estimateBiasByModel[key]; ok {
		m.estimateBiasWeightedPPM -= previous.ppm * previous.weight()
		m.estimateBiasWeights -= previous.weight()
	}
	m.estimateBiasByModel[key] = next
	m.estimateBiasWeightedPPM += next.ppm * next.weight()
	m.estimateBiasWeights += next.weight()
	if m.estimateBiasWeights > 0 {
		m.EstimateBiasPPM.Store(m.estimateBiasWeightedPPM / m.estimateBiasWeights)
	}
}

// EstimateBiasByModel returns a copy of the per-model bias samples.
func (m *Metrics) EstimateBiasByModel() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int64, len(m.estimateBiasByModel))
	for model, sample := range m.estimateBiasByModel {
		out[model] = sample.ppm
	}
	return out
}

// EstimateSamplesByModel returns how many upstream usage reports back each
// model's learned ratio. The learned ratio is only applied after a minimum number
// of samples, so the count is what separates "calibration is in effect" from "not
// enough data yet" - the bias gauge alone cannot express that difference.
func (m *Metrics) EstimateSamplesByModel() map[string]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]uint64, len(m.estimateBiasByModel))
	for model, sample := range m.estimateBiasByModel {
		out[model] = sample.samples
	}
	return out
}