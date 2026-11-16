package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// GlobalEngine is the singleton billing calculator used by all data plane handlers.
var GlobalEngine *BillingEngine

// BillingEngine manages model pricing rates and real-time cost calculation.
type BillingEngine struct {
	repo             *storage.Repository
	mu               sync.RWMutex
	prices           map[string]*storage.ModelPriceRecord
	groupMultipliers map[string]float64
	// wildcard lists every pattern containing "*", sorted most specific first (see
	// patternSpecificity) so the most specific rule always wins instead of
	// depending on Go's randomized map iteration order.
	wildcard []wildcardRule
	deflt    *storage.ModelPriceRecord
	// lastSignature identifies the rate set currently in effect. The periodic
	// sync reloads prices every 10s per replica; without this the log carried an
	// identical "reloaded model pricing engine" line every 10s (about 17k lines
	// per day per replica), which drowned out real billing events.
	lastSignature string
}

// wildcardRule is a pre-sorted wildcard pricing pattern.
type wildcardRule struct {
	group   string
	pattern string
	price   *storage.ModelPriceRecord
}

// NewEngine creates and initializes a BillingEngine with persisted pricing rates.
func NewEngine(repo *storage.Repository) *BillingEngine {
	e := &BillingEngine{
		repo:             repo,
		prices:           make(map[string]*storage.ModelPriceRecord),
		groupMultipliers: map[string]float64{"default": 1.0, "vip": 0.8, "enterprise": 1.5},
		deflt: &storage.ModelPriceRecord{
			Model:           "*",
			PromptPrice:     2.0, // Default benchmark ¥2.00 / 1M prompt
			CompletionPrice: 8.0, // Default benchmark ¥8.00 / 1M completion
			CacheReadPrice:  0.2, // Default benchmark ¥0.20 / 1M cached prompt (90% discount)
			FixedPrice:      0.0,
			Currency:        "CNY",
			// A model with no configured price must not also receive a time-of-use
			// discount: the fallback rate is already an estimate, and halving it
			// (00:00-08:30 plus all weekend for "custom" mode) silently gave away
			// 50% of the revenue for every unpriced model that appeared. Discounts
			// are opt-in per price rule.
			OffPeakEnabled:  false,
			OffPeakMode:     "custom",
			OffPeakDiscount: 0.5,
			WeekendAllDay:   true,
			OffPeakStart:    "00:00",
			OffPeakEnd:      "08:30",
		},
	}
	if repo != nil {
		_ = repo.SeedDefaultModelPrices()
		_ = e.ReloadPrices()
	}
	return e
}

// InitGlobalEngine sets the global billing engine instance.
func InitGlobalEngine(repo *storage.Repository) {
	GlobalEngine = NewEngine(repo)
}

// ReloadPrices loads all pricing rules from the repository into memory.
func (e *BillingEngine) ReloadPrices() error {
	if e.repo == nil {
		return nil
	}
	list, err := e.repo.ListModelPrices()
	if err != nil {
		return err
	}

	m := make(map[string]*storage.ModelPriceRecord)
	for _, rec := range list {
		grp := strings.ToLower(strings.TrimSpace(rec.GroupName))
		if grp == "" {
			grp = "default"
		}
		m[grp+":"+strings.ToLower(rec.Model)] = rec
		if grp == "default" {
			m[strings.ToLower(rec.Model)] = rec
		}
	}

	// Build a deterministic wildcard index: most specific pattern first. Any
	// pattern containing "*" is a wildcard rule - trailing only ("gpt-4o-*") used
	// to be the sole supported form, which made "*o1" and "gpt-4o*mini" savable but
	// permanently unmatched.
	wildcards := make([]wildcardRule, 0)
	for _, rec := range list {
		grp := strings.ToLower(strings.TrimSpace(rec.GroupName))
		if grp == "" {
			grp = "default"
		}
		pattern := strings.ToLower(strings.TrimSpace(rec.Model))
		if strings.Contains(pattern, "*") {
			wildcards = append(wildcards, wildcardRule{
				group:   grp,
				pattern: pattern,
				price:   rec,
			})
		}
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		literalI, starsI := patternSpecificity(wildcards[i].pattern)
		literalJ, starsJ := patternSpecificity(wildcards[j].pattern)
		if literalI != literalJ {
			return literalI > literalJ
		}
		if starsI != starsJ {
			return starsI < starsJ
		}
		return wildcards[i].pattern < wildcards[j].pattern
	})

	multipliers := map[string]float64{"default": 1.0, "vip": 0.8, "enterprise": 1.5}
	if e.repo != nil {
		raw := e.repo.GetSetting("billing_group_multipliers", "")
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &multipliers)
		}
	}
	if _, ok := multipliers["default"]; !ok {
		multipliers["default"] = 1.0
	}

	signature := rateSignature(list, multipliers)

	e.mu.Lock()
	e.prices = m
	e.wildcard = wildcards
	e.groupMultipliers = multipliers
	changed := e.lastSignature != signature
	e.lastSignature = signature
	e.mu.Unlock()

	logger := telemetry.ScopedLogger("billing")
	if changed {
		logger.Info("reloaded model pricing engine", "rules_count", len(list), "groups_count", len(multipliers))
	} else {
		// Periodic reload with identical rates: keep it observable at debug level
		// instead of writing the same INFO line every 10 seconds.
		logger.Debug("model pricing unchanged; reload was a no-op", "rules_count", len(list))
	}
	return nil
}

// rateSignature is a stable digest of the loaded rate card, used only to decide
// whether a periodic reload actually changed anything. Ordering is canonical
// because the rules are sorted before hashing and JSON sorts map keys.
func rateSignature(list []*storage.ModelPriceRecord, multipliers map[string]float64) string {
	hasher := sha256.New()
	encoder := json.NewEncoder(hasher)
	_ = encoder.Encode(list)
	_ = encoder.Encode(multipliers)
	return hex.EncodeToString(hasher.Sum(nil))
}

// GetGroupMultiplier returns the billing rate multiplier for a group (defaults to 1.0).
func (e *BillingEngine) GetGroupMultiplier(groupName string) float64 {
	if e == nil {
		return 1.0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	grp := strings.ToLower(strings.TrimSpace(groupName))
	if grp == "" || grp == "default" {
		return 1.0
	}
	if mult, ok := e.groupMultipliers[grp]; ok && mult > 0 {
		return mult
	}
	return 1.0
}

// GetGroupMultipliers returns a copy of all configured group multipliers.
func (e *BillingEngine) GetGroupMultipliers() map[string]float64 {
	if e == nil {
		return map[string]float64{"default": 1.0}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make(map[string]float64, len(e.groupMultipliers))
	for k, v := range e.groupMultipliers {
		res[k] = v
	}
	if _, ok := res["default"]; !ok {
		res["default"] = 1.0
	}
	return res
}

// SetGroupMultipliers updates in-memory group multipliers and persists to repository.
func (e *BillingEngine) SetGroupMultipliers(multipliers map[string]float64) error {
	if e == nil {
		return nil
	}
	clean := make(map[string]float64)
	clean["default"] = 1.0
	for k, v := range multipliers {
		grp := strings.ToLower(strings.TrimSpace(k))
		if grp != "" && v > 0 {
			clean[grp] = v
		}
	}
	e.mu.Lock()
	e.groupMultipliers = clean
	e.mu.Unlock()

	if e.repo != nil {
		b, err := json.Marshal(clean)
		if err == nil {
			return e.repo.SetSetting("billing_group_multipliers", string(b))
		}
		return err
	}
	return nil
}

// Price resolution sources reported by ResolvePriceSource. "default" means no
// rule matched and the hardcoded fallback rate was applied — the case the startup
// self-check warns about, because that rate is a guess rather than a decision.
const (
	PriceSourceExact    = "exact"
	PriceSourceWildcard = "wildcard"
	PriceSourceDefault  = "default"
)

// GetPriceWithGroup finds the applicable pricing rate for a requested model within a specific pricing tier/group.
func (e *BillingEngine) GetPriceWithGroup(modelName, groupName string) *storage.ModelPriceRecord {
	price, _ := e.resolvePriceWithSource(modelName, groupName)
	return price
}

// ResolvePriceReference returns the same default-group rule and source used by
// billing, so the console does not reimplement wildcard precedence.
func (e *BillingEngine) ResolvePriceReference(modelName string) (*storage.ModelPriceRecord, string) {
	if e == nil {
		return nil, PriceSourceDefault
	}
	return e.resolvePriceWithSource(modelName, "default")
}

// ResolvePriceSource reports how a model's rate was resolved: an exact rule, a
// wildcard rule, or the hardcoded default fallback. Operators use it to find
// models that are being served (and billed) at an unconfigured price.
func (e *BillingEngine) ResolvePriceSource(modelName, groupName string) string {
	if e == nil {
		return PriceSourceDefault
	}
	_, source := e.resolvePriceWithSource(modelName, groupName)
	return source
}
