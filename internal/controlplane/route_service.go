package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// RouteStore is the persistence surface the route rules need.
type RouteStore interface {
	GetModelPriceWithGroup(modelName, groupName string) (*storage.ModelPriceRecord, error)
	ListChannels() ([]*storage.ChannelRecord, error)
	ApplyModelRouteBundle(ctx context.Context, b *storage.ModelRouteBundle) error
}

// RouteSync pushes a route change into the data plane and, when a cluster is
// configured, to its peers. Implemented by *Synchronizer.
type RouteSync interface {
	ReloadAndBroadcast(ctx context.Context, reason string) error
}

// PriceReloader refreshes the billing engine's in-memory rate table.
type PriceReloader interface {
	ReloadPrices() error
}

// Stages of a route save, reported by RouteStageError so the HTTP layer can keep
// a stage-specific message.
const (
	RouteStageRead    = "read_pricing"
	RouteStageCollect = "collect_channels"
	RouteStagePersist = "persist"
	RouteStageApply   = "apply"
)

// RouteStageError reports which stage of a route save failed.
type RouteStageError struct {
	Stage string
	Err   error
}

func (e *RouteStageError) Error() string { return e.Err.Error() }
func (e *RouteStageError) Unwrap() error { return e.Err }

// RouteService owns the model-route save rules: what a valid request is, how the
// price row for the edited group is derived from a partial update, which providers
// are bound to this route, and how the change reaches the data plane.
//
// Both of the computations below used to be private methods on AdminHandler, so
// the only way to exercise them was through an HTTP request against a live
// repository. They are now reachable with fakes.
type RouteService struct {
	store  RouteStore
	sync   RouteSync
	prices PriceReloader
}

// NewRouteService wires the route rules to their dependencies. sync and prices
// may be nil, which is the single-node case with no cluster and no billing
// engine.
func NewRouteService(store RouteStore, sync RouteSync, prices PriceReloader) *RouteService {
	return &RouteService{store: store, sync: sync, prices: prices}
}

// UpdateModelRoute persists a route change and refreshes the data plane.
//
// The write is transactional: fallback, pricing and independent route rows commit together
// or not at all. The refresh steps that follow are NOT part of that transaction.
// A failure after commit is returned as RouteStageApply so the HTTP adapter
// reports "saved, pending application" separately from a rejected write.
func (s *RouteService) UpdateModelRoute(ctx context.Context, req *UpdateModelRouteRequest) error {
	if req == nil {
		return &ValidationError{Message: "模型标识不能为空"}
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return &ValidationError{Message: "模型标识不能为空"}
	}
	req.Model = model

	// Build the complete change set first, then commit it as one unit.
	bundle := &storage.ModelRouteBundle{Model: model}
	if req.FallbackModel != "" && req.FallbackModel != model {
		bundle.FallbackModel = req.FallbackModel
	}

	price, err := s.buildRoutePricing(req)
	if err != nil {
		var stageErr *RouteStageError
		if errors.As(err, &stageErr) {
			return err
		}
		return &ValidationError{Message: err.Error()}
	}
	bundle.Price = price

	providers, err := s.collectRouteProviders(req.ProviderUpdates)
	if err != nil {
		return &RouteStageError{Stage: RouteStageCollect, Err: err}
	}
	bundle.Providers = providers

	if err := s.store.ApplyModelRouteBundle(ctx, bundle); err != nil {
		return &RouteStageError{Stage: RouteStagePersist, Err: err}
	}

	// Refresh in-memory prices so the very next request bills with the new rate.
	// Attempt both reload paths and report any failure after the committed write.
	var applyErr error
	if bundle.Price != nil && s.prices != nil {
		applyErr = s.prices.ReloadPrices()
	}

	if s.sync != nil {
		if err := s.sync.ReloadAndBroadcast(ctx, "model_route_updated"); err != nil {
			applyErr = errors.Join(applyErr, err)
		}
	}
	if applyErr != nil {
		return &RouteStageError{Stage: RouteStageApply, Err: applyErr}
	}

	return nil
}

// buildRoutePricing resolves the price row to persist for the group being
// edited, WITHOUT writing it (the caller commits it with the rest of the route
// change).
//
// Two defects are fixed here:
//  1. It used to load the row with GetModelPrice(), which is hard-wired to the
//     "default" group. Editing a VIP/enterprise route therefore seeded the form
//     from (and wrote back) the default group's rates, silently dragging one
//     group's prices into another.
//  2. It wrote its own transaction, so a later channel failure left the price
//     applied.
func (s *RouteService) buildRoutePricing(req *UpdateModelRouteRequest) (*storage.ModelPriceRecord, error) {
	if req.PromptPrice == nil && req.CompletionPrice == nil && req.CacheReadPrice == nil && req.FixedPrice == nil &&
		req.OffPeakEnabled == nil && req.OffPeakMode == "" && req.OffPeakSlots == "" &&
		req.WeekendAllDay == nil && req.OffPeakStart == "" && req.OffPeakEnd == "" && req.OffPeakDiscount == nil && req.Currency == "" {
		return nil, nil
	}
	currency := req.Currency
	groupName := strings.TrimSpace(req.GroupName)
	if groupName == "" {
		groupName = "default"
	}
	var pPrice, cPrice, cachePrice, fPrice float64
	// Read the exact group first; fall back to the default group only when this
	// group has no row yet (that mirrors how the billing engine resolves rates).
	existing, err := s.store.GetModelPriceWithGroup(req.Model, groupName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, &RouteStageError{Stage: RouteStageRead, Err: err}
	}
	if currency == "" && existing != nil {
		currency = existing.Currency
	}
	if currency == "" {
		currency = "CNY"
	}
	if existing != nil {
		// Start from the stored prices so a partial update (e.g. only the fixed
		// price) cannot silently reset prompt/completion prices to zero.
		pPrice = existing.PromptPrice
		cPrice = existing.CompletionPrice
		cachePrice = existing.CacheReadPrice
		fPrice = existing.FixedPrice
	}
	if req.PromptPrice != nil {
		pPrice = *req.PromptPrice
	}
	if req.CompletionPrice != nil {
		cPrice = *req.CompletionPrice
	}
	if req.CacheReadPrice != nil {
		cachePrice = *req.CacheReadPrice
	}
	if req.FixedPrice != nil {
		fPrice = *req.FixedPrice
	}
	offEnabled := true
	offMode := "custom"
	offSlots := ""
	weekendAll := true
	offStart := "00:00"
	offEnd := "08:30"
	offDiscount := 0.5
	if existing != nil {
		offEnabled = existing.OffPeakEnabled
		offMode = existing.OffPeakMode
		offSlots = existing.OffPeakSlots
		weekendAll = existing.WeekendAllDay
		offStart = existing.OffPeakStart
		offEnd = existing.OffPeakEnd
		offDiscount = existing.OffPeakDiscount
	}
	if req.OffPeakEnabled != nil {
		offEnabled = *req.OffPeakEnabled
	}
	if req.OffPeakMode != "" {
		offMode = req.OffPeakMode
	}
	if req.OffPeakSlots != "" {
		offSlots = req.OffPeakSlots
	}
	if req.WeekendAllDay != nil {
		weekendAll = *req.WeekendAllDay
	}
	if req.OffPeakStart != "" {
		offStart = req.OffPeakStart
	}
	if req.OffPeakEnd != "" {
		offEnd = req.OffPeakEnd
	}
	if req.OffPeakDiscount != nil {
		offDiscount = *req.OffPeakDiscount
	}

	if offEnabled && offSlots != "" {
		if err := billing.ValidateSlotsOverlap(offSlots); err != nil {
			return nil, err
		}
	}

	rec := &storage.ModelPriceRecord{
		Model:           req.Model,
		GroupName:       groupName,
		PromptPrice:     pPrice,
		CompletionPrice: cPrice,
		CacheReadPrice:  cachePrice,
		FixedPrice:      fPrice,
		Currency:        currency,
		OffPeakEnabled:  offEnabled,
		OffPeakMode:     offMode,
		OffPeakSlots:    offSlots,
		WeekendAllDay:   weekendAll,
		OffPeakStart:    offStart,
		OffPeakEnd:      offEnd,
		OffPeakDiscount: offDiscount,
	}
	// The route editor is the second writer of a price row, so it applies the same
	// price validation the pricing screen does.
	if err := ValidatePriceRecord(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// collectRouteProviders plans route-local providers; channel records
// are read only to validate IDs and resolve inherited scheduling defaults.
func (s *RouteService) collectRouteProviders(providerUpdates []ModelProviderUpdate) ([]storage.ModelRouteProvider, error) {
	allChannels, err := s.store.ListChannels()
	if err != nil {
		return nil, err
	}
	if providerUpdates == nil {
		return nil, nil
	}
	byID := make(map[int64]*storage.ChannelRecord, len(allChannels))
	for _, ch := range allChannels {
		byID[ch.ID] = ch
	}
	providers := make([]storage.ModelRouteProvider, 0, len(providerUpdates))
	seen := make(map[int64]bool)
	for _, p := range providerUpdates {
		ch := byID[p.ChannelID]
		if ch == nil {
			return nil, &ValidationError{Message: "所选渠道不存在，请刷新后重试"}
		}
		if seen[p.ChannelID] {
			return nil, &ValidationError{Message: "同一路由不能重复绑定渠道"}
		}
		seen[p.ChannelID] = true
		priority, weight := p.Priority, p.Weight
		if priority <= 0 {
			priority = ch.Priority
		}
		if weight <= 0 {
			weight = ch.Weight
		}
		if priority <= 0 {
			priority = 1
		}
		if weight <= 0 {
			weight = 10
		}
		providers = append(providers, storage.ModelRouteProvider{
			ChannelID: p.ChannelID, Priority: priority, Weight: weight, MappedModel: strings.TrimSpace(p.MappedModel),
		})
	}
	return providers, nil
}

// globalPriceReloader adapts the package-level billing engine to PriceReloader.
//
// The engine is resolved per call rather than captured, because it is a global
// that the CLI and tests may install after the handler is constructed. Keeping
// that read in one adapter is deliberate: it is the only place the route rules
// touch package-level state, which is what lets them be tested with a fake.
type globalPriceReloader struct{}

func (globalPriceReloader) ReloadPrices() error {
	engine := billing.GlobalEngine
	if engine == nil {
		return nil
	}
	return engine.ReloadPrices()
}
