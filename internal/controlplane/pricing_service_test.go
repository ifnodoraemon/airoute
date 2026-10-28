package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// --- fakes ---------------------------------------------------------------

type fakePricingStore struct {
	rates []*storage.ModelPriceRecord

	listErr              error
	saveErr              error
	deleteErr            error
	deleteGroupErr       error
	batchModelsErr       error
	batchItemsErr        error
	saveCalls            int
	deleteCalls          int
	deleteWithGroupCalls int
	batchModelsCalls     int
	batchItemsCalls      int
	lastDeletedModel     string
	lastDeletedGroup     string
	lastBatchModels      []string
	lastBatchItems       []storage.ModelPriceKey
	lastSaved            *storage.ModelPriceRecord
}

func (f *fakePricingStore) ListModelPrices() ([]*storage.ModelPriceRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.rates, nil
}

func (f *fakePricingStore) SaveModelPrice(rec *storage.ModelPriceRecord) error {
	f.saveCalls++
	f.lastSaved = rec
	return f.saveErr
}

func (f *fakePricingStore) DeleteModelPrice(modelName string) error {
	f.deleteCalls++
	f.lastDeletedModel = modelName
	return f.deleteErr
}

func (f *fakePricingStore) DeleteModelPriceWithGroup(modelName, groupName string) error {
	f.deleteWithGroupCalls++
	f.lastDeletedModel = modelName
	f.lastDeletedGroup = groupName
	return f.deleteGroupErr
}

func (f *fakePricingStore) BatchDeleteModelPrices(models []string) (int64, error) {
	f.batchModelsCalls++
	f.lastBatchModels = models
	if f.batchModelsErr != nil {
		return 0, f.batchModelsErr
	}
	return int64(len(models)), nil
}

func (f *fakePricingStore) BatchDeleteModelPriceKeys(keys []storage.ModelPriceKey) (int64, error) {
	f.batchItemsCalls++
	f.lastBatchItems = keys
	if f.batchItemsErr != nil {
		return 0, f.batchItemsErr
	}
	return int64(len(keys)), nil
}

type fakePricingSync struct {
	calls   int
	reasons []string
	err     error
}

func (f *fakePricingSync) ReloadAndBroadcast(_ context.Context, reason string) error {
	f.calls++
	f.reasons = append(f.reasons, reason)
	return f.err
}

type fakeOffPeak struct {
	off      bool
	discount float64
	calls    int
	lastTime time.Time
}

func (f *fakeOffPeak) IsOffPeak(_ *storage.ModelPriceRecord, t time.Time) (bool, float64) {
	f.calls++
	f.lastTime = t
	return f.off, f.discount
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// --- overview ------------------------------------------------------------

func TestPricingServiceOverviewEnrichesOffPeakState(t *testing.T) {
	store := &fakePricingStore{rates: []*storage.ModelPriceRecord{
		{Model: "gpt-x", GroupName: "default", PromptPrice: 1},
		{Model: "gpt-y", GroupName: "default", PromptPrice: 2},
	}}
	offPeak := &fakeOffPeak{off: true, discount: 0.4}
	svc := NewPricingService(store, nil, nil, offPeak)

	got, err := svc.Overview()
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	if len(got.Rates) != 2 {
		t.Fatalf("rates = %d, want 2", len(got.Rates))
	}
	for _, r := range got.Rates {
		if !r.IsCurrentlyOffPeak {
			t.Errorf("%s: IsCurrentlyOffPeak = false, want true", r.Model)
		}
		if r.CurrentDiscount != 0.4 {
			t.Errorf("%s: CurrentDiscount = %v, want 0.4", r.Model, r.CurrentDiscount)
		}
		// The discounted rate the console renders: the stored price times the
		// discount. Without these the column can only show the list price.
		if want := r.ModelPriceRecord.PromptPrice * 0.4; r.EffectivePromptPrice != want {
			t.Errorf("%s: EffectivePromptPrice = %v, want %v", r.Model, r.EffectivePromptPrice, want)
		}
		if want := r.ModelPriceRecord.CompletionPrice * 0.4; r.EffectiveCompletionPrice != want {
			t.Errorf("%s: EffectiveCompletionPrice = %v, want %v", r.Model, r.EffectiveCompletionPrice, want)
		}
		// The embedded record must still be exposed.
		if r.ModelPriceRecord == nil || r.ModelPriceRecord.Model == "" {
			t.Error("the stored record was not embedded in the enriched row")
		}
	}
	if offPeak.calls != 2 {
		t.Errorf("evaluator calls = %d, want one per rate", offPeak.calls)
	}
}

func TestPricingServiceOverviewWithoutEvaluator(t *testing.T) {
	// No billing engine: rates are returned un-enriched rather than failing.
	store := &fakePricingStore{rates: []*storage.ModelPriceRecord{{Model: "gpt-x"}}}
	svc := NewPricingService(store, nil, nil, nil)

	got, err := svc.Overview()
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if len(got.Rates) != 1 {
		t.Fatalf("rates = %d, want 1", len(got.Rates))
	}
	if got.Rates[0].IsCurrentlyOffPeak {
		t.Error("IsCurrentlyOffPeak = true with no evaluator")
	}
	if got.Rates[0].CurrentDiscount != 1.0 {
		t.Errorf("CurrentDiscount = %v, want 1.0", got.Rates[0].CurrentDiscount)
	}
	// With no evaluator the effective rate is the list price, not zero.
	if got.Rates[0].EffectivePromptPrice != got.Rates[0].PromptPrice {
		t.Errorf("EffectivePromptPrice = %v, want the list price %v",
			got.Rates[0].EffectivePromptPrice, got.Rates[0].PromptPrice)
	}
}

func TestPricingServiceOverviewUsesServerTimeZone(t *testing.T) {
	// The console decides "is it off-peak right now" from the server's clock and
	// business time zone, not the browser's, so the derivation is pinned here.
	cases := []struct {
		name        string
		now         time.Time
		wantTime    string
		wantWeekday string
		wantWeekend bool
	}{
		{
			// 02:00 UTC is 10:00 in UTC+8. 2024-01-06 is a Saturday.
			name:        "saturday morning UTC is saturday in CST",
			now:         time.Date(2024, 1, 6, 2, 0, 0, 0, time.UTC),
			wantTime:    "10:00:00",
			wantWeekday: "Saturday",
			wantWeekend: true,
		},
		{
			// 2024-01-08 is a Monday.
			name:        "monday is not a weekend",
			now:         time.Date(2024, 1, 8, 2, 0, 0, 0, time.UTC),
			wantTime:    "10:00:00",
			wantWeekday: "Monday",
			wantWeekend: false,
		},
		{
			// 23:00 UTC on Friday is 07:00 Saturday in UTC+8: the weekday must
			// follow the business zone, not UTC.
			name:        "friday night UTC is saturday in CST",
			now:         time.Date(2024, 1, 5, 23, 0, 0, 0, time.UTC),
			wantTime:    "07:00:00",
			wantWeekday: "Saturday",
			wantWeekend: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakePricingStore{}
			svc := NewPricingService(store, nil, nil, nil)
			svc.now = fixedClock(tc.now)

			got, err := svc.Overview()
			if err != nil {
				t.Fatalf("Overview: %v", err)
			}
			if got.ServerTime != tc.wantTime {
				t.Errorf("ServerTime = %q, want %q", got.ServerTime, tc.wantTime)
			}
			if got.ServerWeekday != tc.wantWeekday {
				t.Errorf("ServerWeekday = %q, want %q", got.ServerWeekday, tc.wantWeekday)
			}
			if got.IsWeekend != tc.wantWeekend {
				t.Errorf("IsWeekend = %v, want %v", got.IsWeekend, tc.wantWeekend)
			}
		})
	}
}

func TestPricingServiceOverviewPropagatesStoreError(t *testing.T) {
	store := &fakePricingStore{listErr: errors.New("db down")}
	svc := NewPricingService(store, nil, nil, nil)

	if _, err := svc.Overview(); err == nil {
		t.Fatal("Overview swallowed the store error")
	}
}

// --- save ----------------------------------------------------------------

func TestPricingServiceSaveRejectsEmptyModel(t *testing.T) {
	for _, in := range []string{"", "   "} {
		store := &fakePricingStore{}
		sync := &fakePricingSync{}
		prices := &fakePriceReloader{}
		svc := NewPricingService(store, sync, prices, nil)

		err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: in})

		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("model %q: err = %v, want *ValidationError", in, err)
		}
		if verr.Message != "模型标识为必填项" {
			t.Errorf("model %q: message = %q", in, verr.Message)
		}
		if store.saveCalls != 0 {
			t.Errorf("model %q: invalid input reached the store", in)
		}
		if prices.calls != 0 || sync.calls != 0 {
			t.Errorf("model %q: invalid input triggered a reload", in)
		}
	}
}

func TestPricingServiceSaveTrimsAndPersists(t *testing.T) {
	store := &fakePricingStore{}
	sync := &fakePricingSync{}
	prices := &fakePriceReloader{}
	svc := NewPricingService(store, sync, prices, nil)

	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "  gpt-x  "})
	if err != nil {
		t.Fatalf("SaveRate: %v", err)
	}

	if store.lastSaved.Model != "gpt-x" {
		t.Errorf("stored model = %q, want the trimmed value", store.lastSaved.Model)
	}
	if prices.calls != 1 {
		t.Errorf("price reload calls = %d, want 1", prices.calls)
	}
	if sync.calls != 1 || sync.reasons[0] != "model_price_updated" {
		t.Errorf("broadcast reasons = %v, want one \"model_price_updated\"", sync.reasons)
	}
}

func TestPricingServiceSaveRejectsOverlappingOffPeakSlots(t *testing.T) {
	store := &fakePricingStore{}
	sync := &fakePricingSync{}
	svc := NewPricingService(store, sync, nil, nil)

	// The overlap rule only applies to "custom", the one mode whose strategy reads
	// OffPeakSlots (billing.ValidateOffPeakSlots gates on the mode).
	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{
		Model: "gpt-x", OffPeakEnabled: true, OffPeakMode: "custom",
		OffPeakSlots: `[{"start":"00:00","end":"10:00","discount":0.5,"days":[1]},{"start":"05:00","end":"12:00","discount":0.5,"days":[1]}]`,
	})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if store.saveCalls != 0 {
		t.Error("an invalid slot definition reached the store")
	}
	if sync.calls != 0 {
		t.Error("an invalid slot definition triggered a broadcast")
	}
}

func TestPricingServiceSaveSkipsSlotValidationWhenOffPeakDisabled(t *testing.T) {
	// The slots are inert while off-peak pricing is off, so they must not block
	// the save.
	store := &fakePricingStore{}
	svc := NewPricingService(store, nil, nil, nil)

	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{
		Model: "gpt-x", OffPeakEnabled: false, OffPeakSlots: "00:00-10:00,05:00-12:00",
	})

	if err != nil {
		t.Fatalf("SaveRate: %v", err)
	}
	if store.saveCalls != 1 {
		t.Errorf("saveCalls = %d, want 1", store.saveCalls)
	}
}

func TestPricingServiceSavePropagatesStoreErrorWithoutReloading(t *testing.T) {
	store := &fakePricingStore{saveErr: errors.New("disk full")}
	sync := &fakePricingSync{}
	prices := &fakePriceReloader{}
	svc := NewPricingService(store, sync, prices, nil)

	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "gpt-x"})

	if err == nil || err.Error() != "disk full" {
		t.Fatalf("err = %v, want the store error", err)
	}
	if prices.calls != 0 || sync.calls != 0 {
		t.Error("a failed write still triggered a reload")
	}
}

// --- delete --------------------------------------------------------------

func TestPricingServiceDeleteTargetsTheRightRow(t *testing.T) {
	t.Run("group scoped", func(t *testing.T) {
		store := &fakePricingStore{}
		sync := &fakePricingSync{}
		svc := NewPricingService(store, sync, nil, nil)

		if err := svc.DeleteRate(context.Background(), "gpt-x", "vip"); err != nil {
			t.Fatalf("DeleteRate: %v", err)
		}
		if store.deleteWithGroupCalls != 1 || store.deleteCalls != 0 {
			t.Errorf("group-scoped delete not used: withGroup=%d plain=%d",
				store.deleteWithGroupCalls, store.deleteCalls)
		}
		if store.lastDeletedGroup != "vip" {
			t.Errorf("group = %q, want vip", store.lastDeletedGroup)
		}
		if sync.calls != 1 || sync.reasons[0] != "model_price_deleted" {
			t.Errorf("broadcast reasons = %v, want one \"model_price_deleted\"", sync.reasons)
		}
	})

	t.Run("all groups", func(t *testing.T) {
		store := &fakePricingStore{}
		svc := NewPricingService(store, nil, nil, nil)

		if err := svc.DeleteRate(context.Background(), "gpt-x", "   "); err != nil {
			t.Fatalf("DeleteRate: %v", err)
		}
		if store.deleteCalls != 1 || store.deleteWithGroupCalls != 0 {
			t.Errorf("blank group should delete across groups: plain=%d withGroup=%d",
				store.deleteCalls, store.deleteWithGroupCalls)
		}
	})

	t.Run("empty model is rejected", func(t *testing.T) {
		store := &fakePricingStore{}
		svc := NewPricingService(store, nil, nil, nil)

		err := svc.DeleteRate(context.Background(), "  ", "vip")

		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("err = %v, want *ValidationError", err)
		}
		if store.deleteCalls != 0 || store.deleteWithGroupCalls != 0 {
			t.Error("an empty model reached the store")
		}
	})
}

// --- batch delete --------------------------------------------------------

func TestPricingServiceBatchDeleteSelection(t *testing.T) {
	t.Run("items take precedence over models", func(t *testing.T) {
		store := &fakePricingStore{}
		sync := &fakePricingSync{}
		svc := NewPricingService(store, sync, nil, nil)

		n, err := svc.BatchDeleteRates(context.Background(),
			[]string{"ignored"},
			[]storage.ModelPriceKey{{Model: "gpt-x", Group: "vip"}})
		if err != nil {
			t.Fatalf("BatchDeleteRates: %v", err)
		}
		if n != 1 {
			t.Errorf("deleted = %d, want 1", n)
		}
		if store.batchItemsCalls != 1 || store.batchModelsCalls != 0 {
			t.Errorf("items should win: items=%d models=%d", store.batchItemsCalls, store.batchModelsCalls)
		}
		if sync.calls != 1 || sync.reasons[0] != "pricing_batch_deleted" {
			t.Errorf("broadcast reasons = %v, want one \"pricing_batch_deleted\"", sync.reasons)
		}
	})

	t.Run("models used when no items", func(t *testing.T) {
		store := &fakePricingStore{}
		svc := NewPricingService(store, nil, nil, nil)

		n, err := svc.BatchDeleteRates(context.Background(), []string{"a", "b"}, nil)
		if err != nil {
			t.Fatalf("BatchDeleteRates: %v", err)
		}
		if n != 2 {
			t.Errorf("deleted = %d, want 2", n)
		}
		if store.batchModelsCalls != 1 || store.batchItemsCalls != 0 {
			t.Errorf("models path not used: items=%d models=%d", store.batchItemsCalls, store.batchModelsCalls)
		}
	})

	t.Run("neither is a caller error, not a silent no-op", func(t *testing.T) {
		store := &fakePricingStore{}
		sync := &fakePricingSync{}
		svc := NewPricingService(store, sync, nil, nil)

		_, err := svc.BatchDeleteRates(context.Background(), nil, nil)

		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("err = %v, want *ValidationError", err)
		}
		if verr.Message != "请提供待删除定价模型列表" {
			t.Errorf("message = %q", verr.Message)
		}
		if store.batchItemsCalls != 0 || store.batchModelsCalls != 0 {
			t.Error("an empty request reached the store")
		}
		if sync.calls != 0 {
			t.Error("an empty request triggered a broadcast")
		}
	})
}

// --- degradation ---------------------------------------------------------

func TestPricingServiceWithoutOptionalDependencies(t *testing.T) {
	store := &fakePricingStore{}
	svc := NewPricingService(store, nil, nil, nil)

	if err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "gpt-x"}); err != nil {
		t.Fatalf("SaveRate without engine or cluster: %v", err)
	}
	if err := svc.DeleteRate(context.Background(), "gpt-x", ""); err != nil {
		t.Fatalf("DeleteRate without engine or cluster: %v", err)
	}
	if _, err := svc.BatchDeleteRates(context.Background(), []string{"gpt-x"}, nil); err != nil {
		t.Fatalf("BatchDeleteRates without engine or cluster: %v", err)
	}
	if _, err := svc.Overview(); err != nil {
		t.Fatalf("Overview without engine or cluster: %v", err)
	}
}

func TestPricingServiceReportsReloadFailures(t *testing.T) {
	// Both refreshes fail. The write committed, so this must not be reported as a
	// rejected save — but it must not be reported as plain success either, because
	// the engine is still charging the previous rates and the peers still hold the
	// previous configuration. That is what HotReloadError expresses.
	store := &fakePricingStore{}
	sync := &fakePricingSync{err: errors.New("redis unreachable")}
	prices := &fakePriceReloader{err: errors.New("engine unavailable")}
	svc := NewPricingService(store, sync, prices, nil)

	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "gpt-x"})

	var hotErr *HotReloadError
	if !errors.As(err, &hotErr) {
		t.Fatalf("err = %v, want *HotReloadError", err)
	}
	if store.saveCalls != 1 {
		t.Errorf("the rate was not persisted: saveCalls = %d", store.saveCalls)
	}
	// Both refreshes are attempted even though the first failed, so one transient
	// failure does not leave the other half of the system stale too.
	if prices.calls != 1 || sync.calls != 1 {
		t.Errorf("both refreshes should have been attempted: prices=%d sync=%d", prices.calls, sync.calls)
	}
}

func TestPricingServiceSucceedsWhenBothRefreshesWork(t *testing.T) {
	// The control: no error when everything picks the change up.
	store := &fakePricingStore{}
	sync := &fakePricingSync{}
	prices := &fakePriceReloader{}
	svc := NewPricingService(store, sync, prices, nil)

	if err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "gpt-x"}); err != nil {
		t.Fatalf("SaveRate: %v", err)
	}
}

func TestGlobalOffPeakEvaluatorToleratesMissingEngine(t *testing.T) {
	off, disc := (globalOffPeakEvaluator{}).IsOffPeak(&storage.ModelPriceRecord{Model: "x"}, time.Now())

	if off {
		t.Error("off-peak reported with no billing engine")
	}
	if disc != 1.0 {
		t.Errorf("discount = %v, want 1.0 with no billing engine", disc)
	}
}
