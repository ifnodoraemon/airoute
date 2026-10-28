package billing

import (
	"math"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// newEstimateTestEngine builds an isolated engine whose rate card is stable, so
// the expected estimates below are arithmetic rather than whatever the seeded
// default prices happen to be.
func newEstimateTestEngine(t *testing.T) *BillingEngine {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := storage.NewRepository(db)
	engine := NewEngine(repo)
	for _, record := range []storage.ModelPriceRecord{
		{Model: "estimate-base", GroupName: "default", PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY"},
		{Model: "estimate-vip", GroupName: "vip", PromptPrice: 10, CompletionPrice: 20, CacheReadPrice: 1, Currency: "CNY"},
		{Model: "estimate-fraction", GroupName: "default", PromptPrice: 0.123456789, CompletionPrice: 0, Currency: "CNY"},
	} {
		record := record
		if err := repo.SaveModelPrice(&record); err != nil {
			t.Fatalf("save price %s/%s: %v", record.GroupName, record.Model, err)
		}
	}
	if err := engine.ReloadPrices(); err != nil {
		t.Fatalf("reload prices: %v", err)
	}
	return engine
}

func useGlobalEngine(t *testing.T, engine *BillingEngine) {
	t.Helper()
	previous := GlobalEngine
	GlobalEngine = engine
	t.Cleanup(func() { GlobalEngine = previous })
}

func TestEstimateCostCNY_PricesPromptAndMaxCompletion(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	// (100000 * 2 + 1000 * 8) / 1e6 = 0.208
	got := EstimateCostCNY("estimate-base", "default", 100_000, 1_000)
	if got != 0.208 {
		t.Fatalf("EstimateCostCNY = %v, want 0.208", got)
	}
}

func TestEstimateCostCNY_PromptOnlyWhenNoCompletionCap(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	for _, maxCompletion := range []int{0, -1} {
		got := EstimateCostCNY("estimate-base", "default", 100_000, maxCompletion)
		if got != 0.2 {
			t.Fatalf("maxCompletionTokens=%d: EstimateCostCNY = %v, want 0.2", maxCompletion, got)
		}
	}
}

func TestEstimateCostCNY_AppliesGroupMultiplierToBaseRate(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	// No vip-specific rule for estimate-base, so the base rate is resolved and the
	// vip multiplier (0.8) applies: 0.208 * 0.8 = 0.1664.
	got := EstimateCostCNY("estimate-base", "vip", 100_000, 1_000)
	if got != 0.1664 {
		t.Fatalf("EstimateCostCNY = %v, want 0.1664", got)
	}
}

func TestEstimateCostCNY_GroupOverrideIsNotMultipliedTwice(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	// A vip-specific rule already carries the vip price; applying the 0.8
	// multiplier again would double-discount it. 1000 * 10 / 1e6 = 0.01.
	got := EstimateCostCNY("estimate-vip", "vip", 1_000, 0)
	if got != 0.01 {
		t.Fatalf("EstimateCostCNY = %v, want 0.01", got)
	}
}

func TestEstimateCostCNY_PricesCacheablePromptAtFullRate(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	// Cache reads are cheaper, but admission cannot know the hit rate, so a
	// conservative bound must charge the whole prompt at the prompt rate (2.0),
	// not the cache-read rate (0.2).
	got := EstimateCostCNY("estimate-base", "default", 1_000_000, 0)
	if got != 2.0 {
		t.Fatalf("EstimateCostCNY = %v, want 2.0 (cache read rate must not be assumed)", got)
	}
}

func TestEstimateCostCNY_RoundsToEightDecimals(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	// 1 * 0.123456789 / 1e6 = 1.23456789e-7, rounded to 1e-8 = 1.2e-7.
	got := EstimateCostCNY("estimate-fraction", "default", 1, 0)
	want := 1.2e-7
	if got != want {
		t.Fatalf("EstimateCostCNY = %v, want %v", got, want)
	}
}

func TestEstimateCostCNY_UsesFallbackRateForUnpricedModel(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	// Unconfigured models settle at the engine's fallback rate (2/8 per 1M); the
	// estimate must use the same card so admission and settlement agree.
	got := EstimateCostCNY("model-that-was-never-priced", "default", 1_000_000, 0)
	if got != 2.0 {
		t.Fatalf("EstimateCostCNY = %v, want fallback rate 2.0", got)
	}
}

func TestEstimateCostCNY_ZeroWhenEngineMissingOrNoTokens(t *testing.T) {
	previous := GlobalEngine
	GlobalEngine = nil
	t.Cleanup(func() { GlobalEngine = previous })

	if got := EstimateCostCNY("estimate-base", "default", 100_000, 1_000); got != 0 {
		t.Fatalf("nil engine: EstimateCostCNY = %v, want 0", got)
	}

	useGlobalEngine(t, newEstimateTestEngine(t))
	if got := EstimateCostCNY("estimate-base", "default", 0, 0); got != 0 {
		t.Fatalf("no tokens: EstimateCostCNY = %v, want 0", got)
	}
	if got := EstimateCostCNY("estimate-base", "default", -10, -10); got != 0 {
		t.Fatalf("negative tokens: EstimateCostCNY = %v, want 0", got)
	}
}

func TestEstimateCostUnits_MatchesEstimateCostCNY(t *testing.T) {
	useGlobalEngine(t, newEstimateTestEngine(t))

	cases := []struct {
		name         string
		model, group string
		prompt, maxC int
		wantUnits    int64
	}{
		{"base group", "estimate-base", "default", 100_000, 1_000, 208_000_000},
		{"group multiplier", "estimate-base", "vip", 100_000, 1_000, 166_400_000},
		{"prompt only", "estimate-base", "default", 1_000_000, 0, 2_000_000_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cny := EstimateCostCNY(tc.model, tc.group, tc.prompt, tc.maxC)
			units := EstimateCostUnits(tc.model, tc.group, tc.prompt, tc.maxC)
			if units != tc.wantUnits {
				t.Fatalf("EstimateCostUnits = %d, want %d", units, tc.wantUnits)
			}
			// The integer form must agree with the CNY form to the sub-unit.
			wantFromCNY := int64(math.Round(cny * 1e9))
			if units != wantFromCNY {
				t.Fatalf("EstimateCostUnits = %d, CNY-derived = %d", units, wantFromCNY)
			}
		})
	}
}

func TestEstimateCostUnits_ZeroCostIsZeroUnits(t *testing.T) {
	previous := GlobalEngine
	GlobalEngine = nil
	t.Cleanup(func() { GlobalEngine = previous })

	if got := EstimateCostUnits("estimate-base", "default", 100_000, 1_000); got != 0 {
		t.Fatalf("nil engine: EstimateCostUnits = %d, want 0", got)
	}
}
