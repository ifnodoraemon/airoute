package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// --- fakes ---------------------------------------------------------------

type fakeRouteStore struct {
	prices   map[string]*storage.ModelPriceRecord
	channels []*storage.ChannelRecord

	lastBundle     *storage.ModelRouteBundle
	lastPriceModel string
	lastPriceGroup string

	getPriceErr error
	listErr     error
	applyErr    error
	applyCalls  int
}

func newFakeRouteStore() *fakeRouteStore {
	return &fakeRouteStore{prices: map[string]*storage.ModelPriceRecord{}}
}

func (f *fakeRouteStore) seedPrice(rec *storage.ModelPriceRecord) {
	f.prices[rec.Model+"|"+rec.GroupName] = rec
}

func (f *fakeRouteStore) GetModelPriceWithGroup(modelName, groupName string) (*storage.ModelPriceRecord, error) {
	f.lastPriceModel, f.lastPriceGroup = modelName, groupName
	if f.getPriceErr != nil {
		return nil, f.getPriceErr
	}
	return f.prices[modelName+"|"+groupName], nil
}

func (f *fakeRouteStore) ListChannels() ([]*storage.ChannelRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.channels, nil
}

func (f *fakeRouteStore) ApplyModelRouteBundle(_ context.Context, b *storage.ModelRouteBundle) error {
	f.applyCalls++
	f.lastBundle = b
	return f.applyErr
}

type fakeRouteSync struct {
	calls   int
	reasons []string
	err     error
}

func (f *fakeRouteSync) ReloadAndBroadcast(_ context.Context, reason string) error {
	f.calls++
	f.reasons = append(f.reasons, reason)
	return f.err
}

type fakePriceReloader struct {
	calls int
	err   error
}

func (f *fakePriceReloader) ReloadPrices() error {
	f.calls++
	return f.err
}

func priceFloat(v float64) *float64 { return &v }

// --- validation ----------------------------------------------------------

func TestRouteServiceRejectsEmptyModel(t *testing.T) {
	for _, in := range []string{"", "   "} {
		store := newFakeRouteStore()
		sync := &fakeRouteSync{}
		svc := NewRouteService(store, sync, nil)

		err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: in})

		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("model %q: err = %v, want *ValidationError", in, err)
		}
		if store.applyCalls != 0 {
			t.Errorf("model %q: invalid request reached the store", in)
		}
		if sync.calls != 0 {
			t.Errorf("model %q: invalid request reloaded the data plane", in)
		}
	}
}

func TestRouteServiceTrimsModelWhitespace(t *testing.T) {
	store := newFakeRouteStore()
	svc := NewRouteService(store, nil, nil)

	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "  gpt-x  "}); err != nil {
		t.Fatalf("UpdateModelRoute: %v", err)
	}
	if store.lastBundle.Model != "gpt-x" {
		t.Errorf("bundle model = %q, want the trimmed value", store.lastBundle.Model)
	}
}

// --- fallback ------------------------------------------------------------

func TestRouteServiceFallbackRule(t *testing.T) {
	t.Run("sets a distinct fallback", func(t *testing.T) {
		store := newFakeRouteStore()
		svc := NewRouteService(store, nil, nil)

		if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
			Model: "gpt-x", FallbackModel: "gpt-y",
		}); err != nil {
			t.Fatalf("UpdateModelRoute: %v", err)
		}
		if store.lastBundle.FallbackModel != "gpt-y" {
			t.Errorf("fallback = %q, want gpt-y", store.lastBundle.FallbackModel)
		}
	})

	t.Run("ignores a self-referential fallback", func(t *testing.T) {
		// A model falling back to itself would be an infinite loop.
		store := newFakeRouteStore()
		svc := NewRouteService(store, nil, nil)

		if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
			Model: "gpt-x", FallbackModel: "gpt-x",
		}); err != nil {
			t.Fatalf("UpdateModelRoute: %v", err)
		}
		if store.lastBundle.FallbackModel != "" {
			t.Errorf("fallback = %q, want empty for a self-reference", store.lastBundle.FallbackModel)
		}
	})
}

// --- pricing -------------------------------------------------------------

func TestRouteServicePriceIsOptional(t *testing.T) {
	store := newFakeRouteStore()
	prices := &fakePriceReloader{}
	svc := NewRouteService(store, nil, prices)

	// No pricing field at all: the request is a routing-only change.
	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "gpt-x"}); err != nil {
		t.Fatalf("UpdateModelRoute: %v", err)
	}
	if store.lastBundle.Price != nil {
		t.Errorf("price = %+v, want nil when no pricing field was sent", store.lastBundle.Price)
	}
	if prices.calls != 0 {
		t.Errorf("pricing was reloaded for a routing-only change: calls = %d", prices.calls)
	}
}

func TestRouteServicePriceReloadsAfterWrite(t *testing.T) {
	store := newFakeRouteStore()
	prices := &fakePriceReloader{}
	svc := NewRouteService(store, nil, prices)

	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model: "gpt-x", PromptPrice: priceFloat(1.5),
	}); err != nil {
		t.Fatalf("UpdateModelRoute: %v", err)
	}
	if store.lastBundle.Price == nil {
		t.Fatal("price row was not part of the bundle")
	}
	if prices.calls != 1 {
		t.Errorf("price reload calls = %d, want 1", prices.calls)
	}
}

func TestRouteServicePriceReloadFailureReportsCommittedWrite(t *testing.T) {
	// The write committed; the previously loaded rates simply stay in effect.
	store := newFakeRouteStore()
	prices := &fakePriceReloader{err: errors.New("engine unavailable")}
	svc := NewRouteService(store, nil, prices)

	err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model: "gpt-x", PromptPrice: priceFloat(1.5),
	})
	var stage *RouteStageError
	if !errors.As(err, &stage) || stage.Stage != RouteStageApply || store.applyCalls != 1 {
		t.Fatalf("want committed write and apply warning, got %v", err)
	}
	if prices.calls != 1 {
		t.Errorf("price reload calls = %d, want 1", prices.calls)
	}
}

func TestRouteServicePriceIsScopedToTheRequestedGroup(t *testing.T) {
	cases := []struct {
		name      string
		group     string
		wantGroup string
	}{
		{"empty group means default", "", "default"},
		{"explicit group is preserved", "vip", "vip"},
		{"group is trimmed", "  vip  ", "vip"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeRouteStore()
			svc := NewRouteService(store, nil, nil)

			if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
				Model: "gpt-x", GroupName: tc.group, PromptPrice: priceFloat(2),
			}); err != nil {
				t.Fatalf("UpdateModelRoute: %v", err)
			}
			if store.lastPriceGroup != tc.wantGroup {
				t.Errorf("read group = %q, want %q", store.lastPriceGroup, tc.wantGroup)
			}
			if store.lastBundle.Price.GroupName != tc.wantGroup {
				t.Errorf("written group = %q, want %q", store.lastBundle.Price.GroupName, tc.wantGroup)
			}
		})
	}
}

func TestRouteServicePartialPriceUpdateKeepsStoredRates(t *testing.T) {
	store := newFakeRouteStore()
	store.seedPrice(&storage.ModelPriceRecord{
		Model: "gpt-x", GroupName: "vip",
		PromptPrice: 10, CompletionPrice: 20, CacheReadPrice: 3, FixedPrice: 4,
		Currency: "CNY", OffPeakEnabled: false, OffPeakMode: "custom",
		OffPeakStart: "01:00", OffPeakEnd: "05:00", OffPeakDiscount: 0.25,
	})
	svc := NewRouteService(store, nil, nil)

	// Only the fixed price is being changed.
	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model: "gpt-x", GroupName: "vip", FixedPrice: priceFloat(9),
	}); err != nil {
		t.Fatalf("UpdateModelRoute: %v", err)
	}

	got := store.lastBundle.Price
	if got.FixedPrice != 9 {
		t.Errorf("fixed price = %v, want 9", got.FixedPrice)
	}
	if got.PromptPrice != 10 || got.CompletionPrice != 20 || got.CacheReadPrice != 3 {
		t.Errorf("omitted rates were reset: prompt=%v completion=%v cache=%v",
			got.PromptPrice, got.CompletionPrice, got.CacheReadPrice)
	}
	if got.OffPeakStart != "01:00" || got.OffPeakEnd != "05:00" || got.OffPeakDiscount != 0.25 {
		t.Errorf("omitted off-peak settings were reset: %+v", got)
	}
}

func TestRouteServiceRejectsInvalidOffPeakSlots(t *testing.T) {
	store := newFakeRouteStore()
	sync := &fakeRouteSync{}
	svc := NewRouteService(store, sync, nil)

	// Two slots that overlap must be refused before anything is written.
	err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model: "gpt-x", OffPeakSlots: "00:00-10:00,05:00-12:00",
	})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if store.applyCalls != 0 {
		t.Error("an invalid slot definition reached the store")
	}
	if sync.calls != 0 {
		t.Error("an invalid slot definition triggered a data plane reload")
	}
}

// --- independent provider bindings ---------------------------------------

func TestRouteServiceCollectChannelUpdates(t *testing.T) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{
		{ID: 1, Name: "chosen", Models: []string{"gpt-x"}, Priority: 4, Weight: 8},
		{ID: 2, Name: "untouched", Models: []string{"gpt-x"}, Priority: 1, Weight: 10},
	}
	svc := NewRouteService(store, nil, nil)
	providers, err := svc.collectRouteProviders([]ModelProviderUpdate{
		{ChannelID: 1, Weight: 11, MappedModel: "gpt-x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].ChannelID != 1 || providers[0].Priority != 4 || providers[0].Weight != 11 || providers[0].MappedModel != "gpt-x" {
		t.Fatalf("unexpected route providers: %+v", providers)
	}
	if store.channels[0].Weight != 8 || len(store.channels[0].Models) != 1 {
		t.Fatal("planning mutated channel capabilities or defaults")
	}
}

func TestRouteServiceCollectIgnoresNonPositivePriorityAndWeight(t *testing.T) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{{ID: 1, Priority: 4, Weight: 8}}
	svc := NewRouteService(store, nil, nil)
	providers, err := svc.collectRouteProviders([]ModelProviderUpdate{{ChannelID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if providers[0].Priority != 4 || providers[0].Weight != 8 {
		t.Fatalf("inherited defaults lost: %+v", providers)
	}
}

func TestRouteServiceCollectDistinguishesOmittedAndEmptyProviders(t *testing.T) {
	svc := NewRouteService(newFakeRouteStore(), nil, nil)
	omitted, err := svc.collectRouteProviders(nil)
	if err != nil || omitted != nil {
		t.Fatalf("omitted policy = %+v, %v", omitted, err)
	}
	empty, err := svc.collectRouteProviders([]ModelProviderUpdate{})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty policy = %+v, %v", empty, err)
	}
}

func TestRouteServiceCollectRejectsMissingOrDuplicateChannels(t *testing.T) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{{ID: 1}}
	svc := NewRouteService(store, nil, nil)
	for _, providers := range [][]ModelProviderUpdate{{{ChannelID: 2}}, {{ChannelID: 1}, {ChannelID: 1}}} {
		_, err := svc.collectRouteProviders(providers)
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("invalid binding was accepted: %v", err)
		}
	}
}

// --- stage reporting -----------------------------------------------------

func TestRouteServiceReportsTheFailingStage(t *testing.T) {
	t.Run("collect", func(t *testing.T) {
		store := newFakeRouteStore()
		store.listErr = errors.New("cannot list channels")
		svc := NewRouteService(store, nil, nil)

		err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "gpt-x"})

		var serr *RouteStageError
		if !errors.As(err, &serr) || serr.Stage != RouteStageCollect {
			t.Fatalf("err = %v, want a RouteStageError for %s", err, RouteStageCollect)
		}
	})

	t.Run("persist", func(t *testing.T) {
		store := newFakeRouteStore()
		store.applyErr = errors.New("constraint violation")
		svc := NewRouteService(store, nil, nil)

		err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "gpt-x"})

		var serr *RouteStageError
		if !errors.As(err, &serr) || serr.Stage != RouteStagePersist {
			t.Fatalf("err = %v, want a RouteStageError for %s", err, RouteStagePersist)
		}
		if !errors.Is(err, store.applyErr) {
			t.Error("the underlying cause is not unwrappable")
		}
	})

	t.Run("apply", func(t *testing.T) {
		store := newFakeRouteStore()
		sync := &fakeRouteSync{err: errors.New("redis unreachable")}
		svc := NewRouteService(store, sync, nil)

		err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "gpt-x"})

		var serr *RouteStageError
		if !errors.As(err, &serr) || serr.Stage != RouteStageApply {
			t.Fatalf("err = %v, want a RouteStageError for %s", err, RouteStageApply)
		}
		// Pins the known ambiguity: the write committed even though the caller is
		// told the request failed. Resolving it belongs to the configuration
		// version work, not to this refactor.
		if store.applyCalls != 1 {
			t.Errorf("applyCalls = %d, want the write to have been attempted", store.applyCalls)
		}
	})
}

func TestRouteServiceWithoutClusterOrEngine(t *testing.T) {
	// Single-node deployment: no synchronizer, no billing engine.
	store := newFakeRouteStore()
	svc := NewRouteService(store, nil, nil)

	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model: "gpt-x", PromptPrice: priceFloat(1),
	}); err != nil {
		t.Fatalf("UpdateModelRoute without optional dependencies: %v", err)
	}
	if store.applyCalls != 1 {
		t.Errorf("applyCalls = %d, want 1", store.applyCalls)
	}
}

func TestGlobalPriceReloaderToleratesMissingEngine(t *testing.T) {
	// The engine is a package global that may not exist (tests, CLI).
	if err := (globalPriceReloader{}).ReloadPrices(); err != nil {
		t.Fatalf("ReloadPrices with no engine: %v", err)
	}
}
