package billing

import (
	"math"
	"sync"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// EstimateCalibrator aligns local token estimation with the upstream tokenizer.
//
// When an upstream reports usage we learn how its tokenizer counts a prompt
// relative to our local chars-per-token guess for that model. Estimates (client
// aborts, upstreams that omit usage) then use the learned ratio, so the amount we
// charge converges on what the provider actually bills. Without this, every
// estimated charge carried the full tokenizer difference - roughly ±10-20% -
// in one direction only, and the error never corrected itself.
//
// The ratio is an exponentially weighted mean per model, clamped to a sane band so
// a few pathological requests cannot swing it.
type EstimateCalibrator struct {
	mu      sync.RWMutex
	samples map[string]*ratioSample
	// persist is invoked every calibrationPersistEvery observations so learned
	// ratios reach durable storage quickly instead of waiting for a timer.
	persist func() error
	pending int
}

// calibrationPersistEvery bounds how many observations may be lost on a crash.
const calibrationPersistEvery = 200

type ratioSample struct {
	ratio float64
	count uint64
}

const (
	// calibrationAlpha weights the newest observation in the running mean.
	calibrationAlpha = 0.1
	// calibrationMinSamples is how many upstream reports are needed before the
	// learned ratio is trusted over the raw heuristic.
	calibrationMinSamples = 5
	// calibrationFloor/Ceiling bound the learned ratio so an outlier burst cannot
	// make estimates absurd.
	calibrationFloor   = 0.4
	calibrationCeiling = 2.5
)

// GlobalCalibrator is the process-wide calibrator used by the API layer.
var GlobalCalibrator = NewEstimateCalibrator()

// NewEstimateCalibrator creates an empty calibrator.
func NewEstimateCalibrator() *EstimateCalibrator {
	return &EstimateCalibrator{samples: make(map[string]*ratioSample)}
}

// Observe records an (upstream actual, local estimate) pair for a model.
func (c *EstimateCalibrator) Observe(modelName string, localEstimate, upstreamActual int) {
	if c == nil || modelName == "" || localEstimate <= 0 || upstreamActual <= 0 {
		return
	}
	ratio := float64(upstreamActual) / float64(localEstimate)
	if ratio < calibrationFloor {
		ratio = calibrationFloor
	}
	if ratio > calibrationCeiling {
		ratio = calibrationCeiling
	}

	c.mu.Lock()
	sample := c.samples[modelName]
	if sample == nil {
		sample = &ratioSample{ratio: ratio}
		c.samples[modelName] = sample
	} else {
		sample.ratio = sample.ratio*(1-calibrationAlpha) + ratio*calibrationAlpha
	}
	sample.count++
	c.pending++
	shouldPersist := c.persist != nil && c.pending >= calibrationPersistEvery
	if shouldPersist {
		c.pending = 0
	}
	persist := c.persist
	c.mu.Unlock()

	if shouldPersist {
		_ = persist()
	}

	// Publish how far the local heuristic is from the upstream tokenizer, so the
	// residual misalignment on estimated charges is visible on /metrics. The
	// per-model value is published alongside a sample-weighted total: storing just
	// this sample in one global gauge made "the estimator bias" depend on whichever
	// model happened to be observed last. The sample count travels with it because
	// Adjust stays silent below calibrationMinSamples - without it an operator
	// cannot tell "ratio is in effect" from "not enough samples yet" - and Ratio is
	// the accessor that owns that pair.
	learnedRatio, learnedSamples := c.Ratio(modelName)
	telemetry.GlobalMetrics.SetEstimateBias(modelName, int64(math.Round((learnedRatio-1)*1e6)), learnedSamples)
}

// Adjust applies the learned ratio to a local estimate. It returns the input
// unchanged until enough upstream samples exist for that model.
//
// The ratio and the sample count are copied *inside* the read lock: ratioSample is
// a shared pointer and Observe mutates its fields under the write lock, so reading
// them after RUnlock raced with a concurrent Observe (the calibrator runs on the
// request path, where both calls are normal).
func (c *EstimateCalibrator) Adjust(modelName string, localEstimate int) int {
	if c == nil || modelName == "" || localEstimate <= 0 {
		return localEstimate
	}
	c.mu.RLock()
	var ratio float64
	var count uint64
	if sample := c.samples[modelName]; sample != nil {
		ratio, count = sample.ratio, sample.count
	}
	c.mu.RUnlock()
	if count < calibrationMinSamples {
		return localEstimate
	}
	adjusted := int(math.Round(float64(localEstimate) * ratio))
	if adjusted < 1 {
		adjusted = 1
	}
	return adjusted
}

// Ratio reports the learned ratio and sample count for a model. Ratio is 1.0
// before any observation. It is the single accessor for that pair: Observe
// publishes both to the estimate-bias/samples gauges through it.
func (c *EstimateCalibrator) Ratio(modelName string) (float64, uint64) {
	if c == nil {
		return 1.0, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if sample := c.samples[modelName]; sample != nil {
		return sample.ratio, sample.count
	}
	return 1.0, 0
}

// RatioState is the persistent form of one model's calibration.
type RatioState struct {
	Ratio   float64 `json:"ratio"`
	Samples uint64  `json:"samples"`
}

// SetPersistHook installs a callback invoked periodically to flush the ratios to
// durable storage.
func (c *EstimateCalibrator) SetPersistHook(hook func() error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.persist = hook
	c.mu.Unlock()
}

// Snapshot returns a copy of the learned ratios, suitable for persistence.
func (c *EstimateCalibrator) Snapshot() map[string]RatioState {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]RatioState, len(c.samples))
	for model, sample := range c.samples {
		out[model] = RatioState{Ratio: sample.ratio, Samples: sample.count}
	}
	return out
}

// Restore loads previously persisted ratios. Existing in-memory samples win when
// they have seen more observations, so a replica that has been running longer is
// not rolled back by a stale snapshot from a peer.
func (c *EstimateCalibrator) Restore(states map[string]RatioState) {
	if c == nil || len(states) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for model, state := range states {
		if state.Ratio <= 0 || state.Samples == 0 {
			continue
		}
		existing := c.samples[model]
		if existing != nil && existing.count >= state.Samples {
			continue
		}
		c.samples[model] = &ratioSample{ratio: state.Ratio, count: state.Samples}
	}
}
