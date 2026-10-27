package controlplane

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// PricingStore is the persistence surface the pricing rules need.
type PricingStore interface {
	ListModelPrices() ([]*storage.ModelPriceRecord, error)
	SaveModelPrice(rec *storage.ModelPriceRecord) error
	DeleteModelPrice(modelName string) error
	DeleteModelPriceWithGroup(modelName, groupName string) error
	BatchDeleteModelPrices(models []string) (int64, error)
	BatchDeleteModelPriceKeys(keys []storage.ModelPriceKey) (int64, error)
}

// PricingSync pushes a pricing change into the data plane and the cluster.
type PricingSync interface {
	ReloadAndBroadcast(ctx context.Context, reason string) error
}

// OffPeakEvaluator reports whether a rate is inside an off-peak window at a
// given time, and the discount that applies. Implemented by *billing.BillingEngine.
type OffPeakEvaluator interface {
	IsOffPeak(p *storage.ModelPriceRecord, t time.Time) (bool, float64)
}

// EnrichedPrice is a stored rate plus its live off-peak state. The embedded
// record keeps the wire shape the console already reads.
//
// The field names here are the console's, not the gateway's own preference. They
// used to be is_current_off_peak / effective_discount while PricingManager read
// is_currently_off_peak / current_discount, so every one of these values arrived
// as undefined and the "live effective rate" column silently showed the standard
// price for every model, off-peak or not. Nothing caught it because no test tied
// the two names together; TestEnrichedPriceFieldNamesAreTheConsoleContract now
// does.
type EnrichedPrice struct {
	*storage.ModelPriceRecord
	IsCurrentlyOffPeak bool    `json:"is_currently_off_peak"`
	CurrentDiscount    float64 `json:"current_discount"`
	// EffectivePromptPrice and EffectiveCompletionPrice are the discounted rates
	// that actually apply right now. The console cannot derive them: only the
	// gateway knows which off-peak window matched.
	EffectivePromptPrice     float64 `json:"effective_prompt_price"`
	EffectiveCompletionPrice float64 `json:"effective_completion_price"`
}

// PricingOverview is the pricing screen's payload. The server clock is part of
// it because the console renders "is off-peak right now" against the server's
// time zone rather than the browser's.
type PricingOverview struct {
	Rates            []EnrichedPrice    `json:"rates"`
	GroupMultipliers map[string]float64 `json:"group_multipliers"`
	ServerTime       string             `json:"server_time"`
	ServerWeekday    string             `json:"server_weekday"`
	IsWeekend        bool               `json:"is_weekend"`
}

// PricingService owns the pricing rules: what a saveable rate is, which row a
// delete targets, how a batch delete picks its selection, and how a change
// reaches the billing engine and the data plane.
//
// The clock and the billing engine are both injected. They used to be read from
// time.Now() and the package-level billing.GlobalEngine inside the handler,
// which is why none of this could be tested deterministically.
type PricingService struct {
	store   PricingStore
	sync    PricingSync
	prices  PriceReloader
	offPeak OffPeakEvaluator
	now     func() time.Time
}

// NewPricingService wires the pricing rules to their dependencies. Every
// dependency may be nil; each use is guarded.
func NewPricingService(store PricingStore, sync PricingSync, prices PriceReloader, offPeak OffPeakEvaluator) *PricingService {
	return &PricingService{
		store:   store,
		sync:    sync,
		prices:  prices,
		offPeak: offPeak,
		now:     time.Now,
	}
}

// roundMoney trims the float noise a multiplication introduces, without losing
// precision for rates that are fractions of a unit. The console displays two
// decimals, but a per-million-token rate can legitimately be far smaller, so the
// value is rounded rather than truncated to a display precision.
func roundMoney(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// Overview returns every rate enriched with its current off-peak state, plus the
// server's local time context.
func (s *PricingService) Overview() (*PricingOverview, error) {
	rates, err := s.store.ListModelPrices()
	if err != nil {
		return nil, err
	}

	now := s.now()
	// The console reasons about off-peak windows in the gateway's business time
	// zone. A minimal image may have no tzdata, so fall back to a fixed offset
	// rather than failing the whole screen.
	loc, errLoc := time.LoadLocation("Asia/Shanghai")
	if errLoc != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	nowCST := now.In(loc)

	enriched := make([]EnrichedPrice, 0, len(rates))
	for _, r := range rates {
		isOff, disc := false, 1.0
		if s.offPeak != nil {
			isOff, disc = s.offPeak.IsOffPeak(r, now)
		}
		enriched = append(enriched, EnrichedPrice{
			ModelPriceRecord:         r,
			IsCurrentlyOffPeak:       isOff,
			CurrentDiscount:          disc,
			EffectivePromptPrice:     roundMoney(r.PromptPrice * disc),
			EffectiveCompletionPrice: roundMoney(r.CompletionPrice * disc),
		})
	}

	weekday := nowCST.Weekday()
	multipliers := map[string]float64{"default": 1.0, "vip": 0.8, "enterprise": 1.5}
	if billing.GlobalEngine != nil {
		multipliers = billing.GlobalEngine.GetGroupMultipliers()
	}

	return &PricingOverview{
		Rates:            enriched,
		GroupMultipliers: multipliers,
		ServerTime:       nowCST.Format("15:04:05"),
		ServerWeekday:    weekday.String(),
		IsWeekend:        weekday == time.Saturday || weekday == time.Sunday,
	}, nil
}

// ValidatePriceRecord rejects a rate that cannot mean anything sensible.
//
// A negative price is refused rather than clamped to zero: it is a client error,
// and silently repairing it would leave the operator believing the rate they sent
// was accepted. This is the opposite of what normalizeModelPrice in storage does
// with an out-of-range off-peak discount, which repairs the value — that field has
// a meaningful default, a price does not.
//
// Both writers of a price row call this: the pricing screen through SaveRate and
// the route editor through RouteService, which builds its own record.
func ValidatePriceRecord(rec *storage.ModelPriceRecord) error {
	if rec == nil {
		return nil
	}
	if err := storage.ValidatePriceCurrency(rec); err != nil {
		return &ValidationError{Message: err.Error()}
	}
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"提示价格", rec.PromptPrice},
		{"补全价格", rec.CompletionPrice},
		{"缓存读取价格", rec.CacheReadPrice},
		{"固定价格", rec.FixedPrice},
	} {
		if f.value < 0 {
			return &ValidationError{Message: f.name + "不能为负数"}
		}
	}
	return nil
}

// SaveRate validates and stores a rate, then refreshes the billing engine and
// the data plane.
func (s *PricingService) SaveRate(ctx context.Context, rec *storage.ModelPriceRecord) error {
	if rec == nil {
		return &ValidationError{Message: "模型标识为必填项"}
	}
	rec.Model = strings.TrimSpace(rec.Model)
	if rec.Model == "" {
		return &ValidationError{Message: "模型标识为必填项"}
	}
	if err := ValidatePriceRecord(rec); err != nil {
		return err
	}
	if rec.OffPeakEnabled && rec.OffPeakSlots != "" {
		if err := billing.ValidateSlotsOverlap(rec.OffPeakSlots); err != nil {
			return &ValidationError{Message: err.Error()}
		}
	}

	if err := s.store.SaveModelPrice(rec); err != nil {
		return err
	}

	return s.applyRateChange(ctx, "model_price_updated")
}

// DeleteRate removes the rate for a model, either within one group or across all
// groups when no group is given.
func (s *PricingService) DeleteRate(ctx context.Context, modelName, groupName string) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return &ValidationError{Message: "模型标识为必填项"}
	}
	groupName = strings.TrimSpace(groupName)

	var err error
	if groupName != "" {
		err = s.store.DeleteModelPriceWithGroup(modelName, groupName)
	} else {
		err = s.store.DeleteModelPrice(modelName)
	}
	if err != nil {
		return err
	}

	return s.applyRateChange(ctx, "model_price_deleted")
}

// BatchDeleteRates removes several rates. A list of model/group keys takes
// precedence over a list of bare model names; supplying neither is a caller
// error rather than a silent no-op.
func (s *PricingService) BatchDeleteRates(ctx context.Context, models []string, items []storage.ModelPriceKey) (int64, error) {
	var n int64
	var err error
	switch {
	case len(items) > 0:
		n, err = s.store.BatchDeleteModelPriceKeys(items)
	case len(models) > 0:
		n, err = s.store.BatchDeleteModelPrices(models)
	default:
		return 0, &ValidationError{Message: "请提供待删除定价模型列表"}
	}
	if err != nil {
		return 0, err
	}

	return n, s.applyRateChange(ctx, "pricing_batch_deleted")
}

// reloadPrices refreshes the billing engine's in-memory rates.
//
// The error is returned rather than discarded. The write has already committed, so
// the caller must not report the save as rejected — but it must not report plain
// success either, because the engine is still charging the previous rates. The
// caller wraps this in a HotReloadError, which is exactly that distinction.
func (s *PricingService) reloadPrices() error {
	if s.prices == nil {
		return nil
	}
	return s.prices.ReloadPrices()
}

// reload pushes the change into the data plane. As with the rate table, a failed
// broadcast is reported rather than swallowed: the peers keep the previous
// configuration until they reload.
func (s *PricingService) reload(ctx context.Context, reason string) error {
	if s.sync == nil {
		return nil
	}
	return s.sync.ReloadAndBroadcast(ctx, reason)
}

// applyRateChange pushes a committed rate change into the in-memory state and
// reports the first thing that failed to pick it up.
//
// Both steps run even if the first fails, so one transient failure does not leave
// the other half of the system stale as well.
func (s *PricingService) applyRateChange(ctx context.Context, reason string) error {
	priceErr := s.reloadPrices()
	syncErr := s.reload(ctx, reason)

	if priceErr != nil {
		return &HotReloadError{Stage: "计费引擎费率", Err: priceErr}
	}
	if syncErr != nil {
		return &HotReloadError{Stage: "数据面集群广播", Err: syncErr}
	}
	return nil
}

// globalOffPeakEvaluator adapts the package-level billing engine to
// OffPeakEvaluator, resolving it per call for the same reason as
// globalPriceReloader: the global may be installed after the handler is built.
type globalOffPeakEvaluator struct{}

func (globalOffPeakEvaluator) IsOffPeak(p *storage.ModelPriceRecord, t time.Time) (bool, float64) {
	engine := billing.GlobalEngine
	if engine == nil {
		return false, 1.0
	}
	return engine.IsOffPeak(p, t)
}
