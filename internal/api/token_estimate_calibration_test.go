package api

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// withFreshCalibrator swaps the process-wide calibrator for an isolated one.
func withFreshCalibrator(t *testing.T) *billing.EstimateCalibrator {
	t.Helper()
	previous := billing.GlobalCalibrator
	calibrator := billing.NewEstimateCalibrator()
	billing.GlobalCalibrator = calibrator
	t.Cleanup(func() { billing.GlobalCalibrator = previous })
	return calibrator
}

// C-04: the embedding and rerank estimators must use the same learned tokenizer
// ratio as every chat estimate, instead of the raw chars/4 heuristic.
func TestModalityEstimatesUseCalibration(t *testing.T) {
	calibrator := withFreshCalibrator(t)
	const modelName = "text-embedding-3-small"

	req := &model.EmbeddingRequest{Model: modelName, Input: "0123456789"}
	baseline, source := resolveEmbeddingTokens(req, model.EmbeddingUsage{})
	if source != usageSourceEstimated {
		t.Fatalf("source = %q, want estimated", source)
	}

	// Teach the calibrator that the upstream tokenizer counts twice as many
	// tokens as the local heuristic for this model.
	for i := 0; i < 6; i++ {
		calibrator.Observe(modelName, 10, 20)
	}
	want := calibrator.Adjust(modelName, estimateTokens(10))
	got, source := resolveEmbeddingTokens(req, model.EmbeddingUsage{})
	if source != usageSourceEstimated {
		t.Fatalf("source = %q, want estimated", source)
	}
	if got != want {
		t.Errorf("embedding estimate = %d, want calibrated %d (baseline %d)", got, want, baseline)
	}
	if got <= baseline {
		t.Errorf("calibration did not raise the estimate (%d <= %d)", got, baseline)
	}
}

// A real upstream count must feed the calibrator: that report is the only signal
// the ratio is learned from.
func TestUpstreamUsageFeedsCalibrator(t *testing.T) {
	calibrator := withFreshCalibrator(t)
	const modelName = "rerank-model"

	req := &model.RerankRequest{Model: modelName, Query: "query", Documents: []any{"doc"}}
	if _, source := resolveRerankTokens(req, model.RerankUsage{TotalTokens: 40}); source != usageSourceUpstream {
		t.Fatalf("source = %q, want upstream", source)
	}
	if ratio, samples := calibrator.Ratio(modelName); samples == 0 || ratio == 0 {
		t.Fatalf("upstream usage did not reach the calibrator (ratio=%v samples=%d)", ratio, samples)
	}
}

// Token-id inputs are already exact counts and must not be calibrated twice.
func TestEmbeddingTokenIDInputIsNotCalibrated(t *testing.T) {
	calibrator := withFreshCalibrator(t)
	const modelName = "text-embedding-3-small"
	for i := 0; i < 6; i++ {
		calibrator.Observe(modelName, 10, 20)
	}

	req := &model.EmbeddingRequest{Model: modelName, Input: []any{float64(1), float64(2), float64(3)}}
	got, source := resolveEmbeddingTokens(req, model.EmbeddingUsage{})
	if source != usageSourceEstimated {
		t.Fatalf("source = %q, want estimated", source)
	}
	if got != 3 {
		t.Errorf("token-id estimate = %d, want 3 (must not be calibrated)", got)
	}
}
