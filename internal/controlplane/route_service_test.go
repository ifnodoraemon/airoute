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

func TestRouteServicePriceReloadFailureDoesNotFailTheRequest(t *testing.T) {
	// The write committed; the previously loaded rates simply stay in effect.
	store := newFakeRouteStore()
	prices := &fakePriceReloader{err: errors.New("engine unavailable")}
	svc := NewRouteService(store, nil, prices)

	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model: "gpt-x", PromptPrice: priceFloat(1.5),
	}); err != nil {
		t.Fatalf("price reload failure leaked out of the request: %v", err)
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

// --- channel collection --------------------------------------------------

func TestRouteServiceCollectChannelUpdates(t *testing.T) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{
		// Opted in: already serves the model, but weight changes.
		{ID: 1, Name: "opted-in-weight", Models: []string{"gpt-x"}, Priority: 1, Weight: 5},
		// Opted in: does not serve the model yet, and maps it elsewhere.
		{ID: 2, Name: "opted-in-new", Models: []string{"other"}, Priority: 1, Weight: 5},
		// Opted in: already exactly as requested.
		{ID: 3, Name: "already-correct", Models: []string{"gpt-x"}, Priority: 2, Weight: 9,
			ModelMapping: map[string]string{"gpt-x": "upstream-x"}},
		// Not listed: serves the model and must be opted out.
		{ID: 4, Name: "opted-out", Models: []string{"gpt-x", "keep"}, Priority: 1, Weight: 5,
			ModelMapping: map[string]string{"gpt-x": "old", "keep": "keep"}},
		// Not listed and unaffected.
		{ID: 5, Name: "untouched", Models: []string{"other"}, Priority: 1, Weight: 5},
	}
	svc := NewRouteService(store, nil, nil)

	updates, err := svc.collectChannelUpdatesForModel("gpt-x", []ModelProviderUpdate{
		{ChannelID: 1, Weight: 11},
		{ChannelID: 2, MappedModel: "upstream-x"},
		{ChannelID: 3, Priority: 2, Weight: 9, MappedModel: "upstream-x"},
	})
	if err != nil {
		t.Fatalf("collectChannelUpdatesForModel: %v", err)
	}

	byID := map[int64]storage.ChannelRoutingUpdate{}
	for _, u := range updates {
		byID[u.ID] = u
	}

	// Every channel is described. "Ensure the model is present" is idempotent, so
	// stating it for a channel that already satisfies it costs nothing and cannot
	// lose a model another administrator added in between. The storage layer skips
	// the write when a delta turns out to be a no-op.
	if len(updates) != len(store.channels) {
		t.Fatalf("updates = %d, want one per channel (%d)", len(updates), len(store.channels))
	}

	one := byID[1]
	if one.Weight != 11 {
		t.Errorf("channel 1 weight = %d, want 11", one.Weight)
	}
	if len(one.AddModels) != 1 || one.AddModels[0] != "gpt-x" {
		t.Errorf("channel 1 AddModels = %v, want [gpt-x]", one.AddModels)
	}

	two := byID[2]
	if len(two.AddModels) != 1 || two.AddModels[0] != "gpt-x" {
		t.Errorf("channel 2 AddModels = %v, want [gpt-x]", two.AddModels)
	}
	if two.SetMapping["gpt-x"] != "upstream-x" {
		t.Errorf("channel 2 SetMapping = %v, want gpt-x -> upstream-x", two.SetMapping)
	}

	three := byID[3]
	if len(three.RemoveMapping) != 0 {
		t.Errorf("channel 3 should keep its mapping, got RemoveMapping = %v", three.RemoveMapping)
	}
	if three.SetMapping["gpt-x"] != "upstream-x" {
		t.Errorf("channel 3 SetMapping = %v", three.SetMapping)
	}

	four := byID[4]
	if len(four.RemoveModels) != 1 || four.RemoveModels[0] != "gpt-x" {
		t.Errorf("opted-out channel RemoveModels = %v, want [gpt-x]", four.RemoveModels)
	}
	if len(four.RemoveMapping) != 1 || four.RemoveMapping[0] != "gpt-x" {
		t.Errorf("opted-out channel RemoveMapping = %v, want [gpt-x]", four.RemoveMapping)
	}
	// The delta names only gpt-x, so the unrelated "keep" mapping is never touched.
	for _, k := range four.RemoveMapping {
		if k == "keep" {
			t.Error("the delta would remove an unrelated mapping")
		}
	}

	five := byID[5]
	if len(five.RemoveModels) != 1 || five.RemoveModels[0] != "gpt-x" {
		t.Errorf("untouched channel RemoveModels = %v, want [gpt-x]", five.RemoveModels)
	}
}

func TestRouteServiceCollectIgnoresNonPositivePriorityAndWeight(t *testing.T) {
	// 0 means "leave as is" (the same convention the channel rules use), so a
	// provider update of 0 must not zero out a live routing weight.
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{
		{ID: 1, Name: "chan", Models: []string{"gpt-x"}, Priority: 4, Weight: 8},
	}
	svc := NewRouteService(store, nil, nil)

	updates, err := svc.collectChannelUpdatesForModel("gpt-x", []ModelProviderUpdate{
		{ChannelID: 1, Priority: 0, Weight: 0},
	})
	if err != nil {
		t.Fatalf("collectChannelUpdatesForModel: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(updates))
	}
	// The delta omits both values, so the stored 4/8 survive. Carrying them as
	// zeroes would reset a live routing weight, which is what this pins.
	if updates[0].Priority != 0 || updates[0].Weight != 0 {
		t.Errorf("delta carried Priority=%d Weight=%d, want both unset",
			updates[0].Priority, updates[0].Weight)
	}
	// And the model itself is still requested, so a concurrent removal cannot win.
	if len(updates[0].AddModels) != 1 || updates[0].AddModels[0] != "gpt-x" {
		t.Errorf("AddModels = %v, want [gpt-x]", updates[0].AddModels)
	}
}

func TestRouteServiceCollectClearsMappingWhenMappedModelIsEmpty(t *testing.T) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{
		{ID: 1, Name: "chan", Models: []string{"gpt-x"}, Priority: 1, Weight: 5,
			ModelMapping: map[string]string{"gpt-x": "upstream-x"}},
	}
	svc := NewRouteService(store, nil, nil)

	updates, err := svc.collectChannelUpdatesForModel("gpt-x", []ModelProviderUpdate{{ChannelID: 1}})
	if err != nil {
		t.Fatalf("collectChannelUpdatesForModel: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(updates))
	}
	// An empty MappedModel means "no remap", so the delta asks for the mapping key
	// to be dropped rather than rewriting a mapping map.
	if len(updates[0].RemoveMapping) != 1 || updates[0].RemoveMapping[0] != "gpt-x" {
		t.Errorf("RemoveMapping = %v, want [gpt-x]", updates[0].RemoveMapping)
	}
	if len(updates[0].SetMapping) != 0 {
		t.Errorf("SetMapping = %v, want empty", updates[0].SetMapping)
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
