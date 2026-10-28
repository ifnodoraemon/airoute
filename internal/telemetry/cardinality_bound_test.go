package telemetry

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// BUG-DATA-05 regression: the per-model estimate-bias metric is bounded like the
// sibling per-model request metric. Model names come from the client's request, so
// an unbounded map would be an unbounded memory leak and an unbounded Prometheus
// label set.
func TestEstimateBiasModelLabelsAreBounded(t *testing.T) {
	m := &Metrics{}
	const models = 5000
	for i := 0; i < models; i++ {
		name := "client-model-" + strings.Repeat("x", i%7) + "-" + strconv.Itoa(i)
		m.RecordRequestWithModel(name, true, time.Millisecond, 1, 1)
		m.SetEstimateBias(name, 1000, 10)
	}

	m.mu.RLock()
	requestModels := len(m.modelRequests)
	biasModels := len(m.estimateBiasByModel)
	overflow, overflowOK := m.estimateBiasByModel[modelLabelOverflow]
	m.mu.RUnlock()

	// Both label maps saturate at the cap plus the single overflow bucket.
	if requestModels > modelLabelCap+1 {
		t.Fatalf("modelRequests = %d entries, want <= %d (cap + overflow)", requestModels, modelLabelCap+1)
	}
	if biasModels > modelLabelCap+1 {
		t.Fatalf("estimateBiasByModel = %d entries, want <= %d (cap + overflow)", biasModels, modelLabelCap+1)
	}
	if !overflowOK {
		t.Fatalf("overflow bucket %q missing after %d models", modelLabelOverflow, models)
	}
	// Every folded model contributed 10 samples to the single overflow bucket.
	if overflow.samples != (models-modelLabelCap)*10 {
		t.Fatalf("overflow bucket samples = %d, want %d (10 samples per folded model)", overflow.samples, (models-modelLabelCap)*10)
	}

	// The exported series are bounded too: the label set can never grow past the cap.
	body := m.ToPrometheusFormat()
	for _, prefix := range []string{
		"airoute_billing_estimate_bias_ppm{model=",
		"airoute_billing_estimate_samples{model=",
	} {
		if got := strings.Count(body, prefix); got > modelLabelCap+1 {
			t.Fatalf("%s exported %d series, want <= %d", prefix, got, modelLabelCap+1)
		}
	}
	if !strings.Contains(body, `{model="_unrecognized"}`) {
		t.Fatal("overflow bucket is not exported")
	}
}

// The overflow bucket keeps a sample-weighted merge of the folded models instead of
// letting whichever model was observed last overwrite it, and the unlabelled mean
// stays the sample-weighted mean of the whole (bounded) map.
func TestEstimateBiasOverflowMergesSampleWeighted(t *testing.T) {
	m := &Metrics{}
	for i := 0; i < modelLabelCap; i++ {
		m.SetEstimateBias("model-"+strconv.Itoa(i), 1000, 1)
	}
	if got := len(m.EstimateBiasByModel()); got != modelLabelCap {
		t.Fatalf("map holds %d models at the cap, want %d", got, modelLabelCap)
	}

	// Two models past the cap fold into the overflow bucket:
	//   (2000 ppm, 2 samples) then (-1000 ppm, 1 sample)
	//   merged ppm = (2000*2 + -1000*1) / 3 = 1000, samples = 3
	m.SetEstimateBias("overflow-a", 2000, 2)
	m.SetEstimateBias("overflow-b", -1000, 1)

	bias := m.EstimateBiasByModel()
	samples := m.EstimateSamplesByModel()
	if _, ok := bias["overflow-a"]; ok {
		t.Fatal("a model past the cap became its own label")
	}
	if bias[modelLabelOverflow] != 1000 {
		t.Fatalf("overflow ppm = %d, want 1000 (sample-weighted merge)", bias[modelLabelOverflow])
	}
	if samples[modelLabelOverflow] != 3 {
		t.Fatalf("overflow samples = %d, want 3", samples[modelLabelOverflow])
	}

	// Unlabelled gauge = weighted mean over the bounded map:
	//   (500 models * 1000 ppm * 1 sample + 1000 ppm * 3 samples) / (500 + 3) = 1000
	if got := m.EstimateBiasPPM.Load(); got != 1000 {
		t.Fatalf("EstimateBiasPPM = %d, want 1000 (sample-weighted mean)", got)
	}
}

// Updating the same model repeatedly must keep the incremental weighted mean exact
// (the O(1) bookkeeping subtracts the old value before adding the new one).
func TestEstimateBiasIncrementalMeanStaysExact(t *testing.T) {
	m := &Metrics{}

	m.SetEstimateBias("a", 200000, 7)
	m.SetEstimateBias("b", -100000, 3)
	// (200000*7 + -100000*3) / 10 = 110000
	if got := m.EstimateBiasPPM.Load(); got != 110000 {
		t.Fatalf("EstimateBiasPPM = %d, want 110000", got)
	}

	// Replace a's sample: (500000*2 + -100000*3) / 5 = 140000
	m.SetEstimateBias("a", 500000, 2)
	if got := m.EstimateBiasPPM.Load(); got != 140000 {
		t.Fatalf("EstimateBiasPPM after replacement = %d, want 140000", got)
	}

	// Zero-sample observations carry weight 1 instead of disappearing from the mean.
	m.SetEstimateBias("c", 50000, 0)
	// (500000*2 + -100000*3 + 50000*1) / 6 = 74166 (integer division)
	if got, want := m.EstimateBiasPPM.Load(), int64((500000*2-100000*3+50000*1)/6); got != want {
		t.Fatalf("EstimateBiasPPM with zero-sample model = %d, want %d", got, want)
	}

	// An empty model name is ignored and never becomes a label.
	m.SetEstimateBias("", 999999, 1)
	if _, ok := m.EstimateBiasByModel()[""]; ok {
		t.Fatal("empty model name was stored as a label")
	}
}
