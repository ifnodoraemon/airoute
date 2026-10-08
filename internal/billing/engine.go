package billing

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// GlobalEngine is the singleton billing calculator used by all data plane handlers.
var GlobalEngine *BillingEngine

// Cached Asia/Shanghai timezone to avoid repeated disk I/O on every request
var shanghaiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

// OffPeakSlot represents a discrete discount time window.
type OffPeakSlot struct {
	Start    string  `json:"start"`          // e.g. "00:00"
	End      string  `json:"end"`            // e.g. "08:30"
	Discount float64 `json:"discount"`       // e.g. 0.5
	Name     string  `json:"name,omitempty"` // e.g. "夜间优惠"
	Days     []int   `json:"days,omitempty"` // 1=Mon, 2=Tue, 3=Wed, 4=Thu, 5=Fri, 6=Sat, 7=Sun (empty = all days)
}

// OffPeakEvaluationContext carries time context for strategy evaluation.
type OffPeakEvaluationContext struct {
	LocalTime time.Time
	Weekday   time.Weekday
	IsWeekend bool
	ISODay    int
	CurrentM  int
	Discount  float64
}

// OffPeakStrategy defines the strategy interface for off-peak discount computation.
type OffPeakStrategy interface {
	Evaluate(p *storage.ModelPriceRecord, ctx *OffPeakEvaluationContext) (bool, float64)
}

// NightOffPeakStrategy evaluates night-time discounts.
type NightOffPeakStrategy struct{}

func (s *NightOffPeakStrategy) Evaluate(p *storage.ModelPriceRecord, ctx *OffPeakEvaluationContext) (bool, float64) {
	startM := parseTimeToMinutes(p.OffPeakStart)
	endM := parseTimeToMinutes(p.OffPeakEnd)
	if startM == 0 && endM == 0 {
		startM = 0
		endM = 510 // 08:30
	}
	var isOff bool
	if startM <= endM {
		isOff = ctx.CurrentM >= startM && ctx.CurrentM < endM
	} else {
		isOff = ctx.CurrentM >= startM || ctx.CurrentM < endM
	}
	if isOff {
		return true, ctx.Discount
	}
	return false, 1.0
}

// CustomOffPeakStrategy evaluates configurable multi-slot time and day-of-week discounts.
type CustomOffPeakStrategy struct{}

func (s *CustomOffPeakStrategy) Evaluate(p *storage.ModelPriceRecord, ctx *OffPeakEvaluationContext) (bool, float64) {
	bestDiscount := 1.0
	matched := false

	// 1. Weekend all-day evaluation if enabled
	if p.WeekendAllDay && ctx.IsWeekend {
		matched = true
		bestDiscount = ctx.Discount
	}

	// 2. Parse slots
	var slots []OffPeakSlot
	if p.OffPeakSlots != "" {
		_ = json.Unmarshal([]byte(p.OffPeakSlots), &slots)
	}

	// 3. Evaluate each slot with conflict resolution: Best Discount Rule (lowest discount multiplier)
	for _, slot := range slots {
		if len(slot.Days) > 0 {
			dayMatch := false
			for _, d := range slot.Days {
				if d == ctx.ISODay {
					dayMatch = true
					break
				}
			}
			if !dayMatch {
				continue
			}
		}

		sM := parseTimeToMinutes(slot.Start)
		eM := parseTimeToMinutes(slot.End)
		d := slot.Discount
		if d <= 0 || d >= 1.0 {
			d = ctx.Discount
		}

		var inSlot bool
		if sM <= eM {
			inSlot = ctx.CurrentM >= sM && ctx.CurrentM < eM
		} else {
			inSlot = ctx.CurrentM >= sM || ctx.CurrentM < eM
		}

		if inSlot {
			matched = true
			if d < bestDiscount {
				bestDiscount = d
			}
		}
	}

	// 4. If no custom multi-slots configured, evaluate default start/end window
	if len(slots) == 0 && (p.OffPeakStart != "" || p.OffPeakEnd != "") {
		startM := parseTimeToMinutes(p.OffPeakStart)
		endM := parseTimeToMinutes(p.OffPeakEnd)
		if startM == 0 && endM == 0 {
			startM = 0
			endM = 510 // 08:30
		}
		var inWindow bool
		if startM <= endM {
			inWindow = ctx.CurrentM >= startM && ctx.CurrentM < endM
		} else {
			inWindow = ctx.CurrentM >= startM || ctx.CurrentM < endM
		}
		if inWindow {
			matched = true
			if ctx.Discount < bestDiscount {
				bestDiscount = ctx.Discount
			}
		}
	}

	if matched {
		return true, bestDiscount
	}
	return false, 1.0
}

// NoneOffPeakStrategy disables off-peak discounts.
type NoneOffPeakStrategy struct{}

func (s *NoneOffPeakStrategy) Evaluate(p *storage.ModelPriceRecord, ctx *OffPeakEvaluationContext) (bool, float64) {
	return false, 1.0
}

// WindowOffPeakStrategy handles default start/end single window discounts.
type WindowOffPeakStrategy struct{}

func (s *WindowOffPeakStrategy) Evaluate(p *storage.ModelPriceRecord, ctx *OffPeakEvaluationContext) (bool, float64) {
	startM := parseTimeToMinutes(p.OffPeakStart)
	endM := parseTimeToMinutes(p.OffPeakEnd)
	var isOff bool
	if startM <= endM {
		isOff = ctx.CurrentM >= startM && ctx.CurrentM < endM
	} else {
		isOff = ctx.CurrentM >= startM || ctx.CurrentM < endM
	}
	if isOff {
		return true, ctx.Discount
	}
	return false, 1.0
}

var offPeakStrategies = map[string]OffPeakStrategy{
	"night":  &NightOffPeakStrategy{},
	"custom": &CustomOffPeakStrategy{},
	"none":   &NoneOffPeakStrategy{},
}

// BillingEngine manages model pricing rates and real-time cost calculation.
type BillingEngine struct {
	repo   *storage.Repository
	mu     sync.RWMutex
	prices map[string]*storage.ModelPriceRecord
	deflt  *storage.ModelPriceRecord
}

// NewEngine creates and initializes a BillingEngine with persisted pricing rates.
func NewEngine(repo *storage.Repository) *BillingEngine {
	e := &BillingEngine{
		repo:   repo,
		prices: make(map[string]*storage.ModelPriceRecord),
		deflt: &storage.ModelPriceRecord{
			Model:           "*",
			PromptPrice:     2.0, // Default benchmark ¥2.00 / 1M prompt
			CompletionPrice: 8.0, // Default benchmark ¥8.00 / 1M completion
			CacheReadPrice:  0.2, // Default benchmark ¥0.20 / 1M cached prompt (90% discount)
			FixedPrice:      0.0,
			Currency:        "CNY",
			OffPeakEnabled:  true,
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

	e.mu.Lock()
	e.prices = m
	e.mu.Unlock()

	telemetry.Logger.Info("reloaded model pricing engine", "rules_count", len(list))
	return nil
}

// GetPrice finds the applicable pricing rate for a requested model in default group.
func (e *BillingEngine) GetPrice(modelName string) *storage.ModelPriceRecord {
	return e.GetPriceWithGroup(modelName, "default")
}

// GetPriceWithGroup finds the applicable pricing rate for a requested model within a specific pricing tier/group.
func (e *BillingEngine) GetPriceWithGroup(modelName, groupName string) *storage.ModelPriceRecord {
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	grp := strings.ToLower(strings.TrimSpace(groupName))
	if grp == "" {
		grp = "default"
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Exact match in requested group
	if p, ok := e.prices[grp+":"+modelLower]; ok {
		return p
	}

	// 2. Exact match in default group fallback
	if grp != "default" {
		if p, ok := e.prices["default:"+modelLower]; ok {
			return p
		}
		if p, ok := e.prices[modelLower]; ok {
			return p
		}
	} else {
		if p, ok := e.prices[modelLower]; ok {
			return p
		}
	}

	// 3. Wildcard prefix match in group (e.g. "vip:gpt-4o-*" matching "gpt-4o-mini")
	prefixMatch := func(g string) *storage.ModelPriceRecord {
		prefixKey := g + ":"
		for k, p := range e.prices {
			if strings.HasPrefix(k, prefixKey) {
				pattern := strings.TrimPrefix(k, prefixKey)
				if strings.HasSuffix(pattern, "*") {
					patPrefix := strings.TrimSuffix(pattern, "*")
					if strings.HasPrefix(modelLower, patPrefix) {
						return p
					}
				}
			}
		}
		return nil
	}

	if p := prefixMatch(grp); p != nil {
		return p
	}
	if grp != "default" {
		if p := prefixMatch("default"); p != nil {
			return p
		}
	}

	// 4. Group wildcard or global wildcard "*"
	if p, ok := e.prices[grp+":*"]; ok {
		return p
	}
	if p, ok := e.prices["*"]; ok {
		return p
	}

	// 5. Default standard fallback
	return e.deflt
}

// IsOffPeak evaluates whether the specified time is within the model's off-peak time window.
// Uses Strategy Pattern across modes: "custom", "night", "none", and single-window default.
func (e *BillingEngine) IsOffPeak(p *storage.ModelPriceRecord, t time.Time) (bool, float64) {
	if p == nil || !p.OffPeakEnabled {
		return false, 1.0
	}

	discount := p.OffPeakDiscount
	if discount <= 0 || discount >= 1.0 {
		discount = 0.5
	}

	localT := t.In(shanghaiLoc)
	weekday := localT.Weekday()
	isoDay := int(weekday)
	if isoDay == 0 {
		isoDay = 7
	}

	evalCtx := &OffPeakEvaluationContext{
		LocalTime: localT,
		Weekday:   weekday,
		IsWeekend: (weekday == time.Saturday || weekday == time.Sunday),
		ISODay:    isoDay,
		CurrentM:  localT.Hour()*60 + localT.Minute(),
		Discount:  discount,
	}

	mode := strings.ToLower(strings.TrimSpace(p.OffPeakMode))
	strategy, exists := offPeakStrategies[mode]
	if !exists {
		strategy = &WindowOffPeakStrategy{}
	}

	return strategy.Evaluate(p, evalCtx)
}

// ValidateSlotsOverlap ensures discrete time windows do not overlap on the same day of the week.
func ValidateSlotsOverlap(slotsJSON string) error {
	if strings.TrimSpace(slotsJSON) == "" {
		return nil
	}
	var slots []OffPeakSlot
	if err := json.Unmarshal([]byte(slotsJSON), &slots); err != nil {
		return fmt.Errorf("优惠时段格式无效: %w", err)
	}
	if len(slots) <= 1 {
		return nil
	}

	type interval struct {
		start int
		end   int
	}

	getIntervals := func(sStr, eStr string) ([]interval, error) {
		sM := parseTimeToMinutes(sStr)
		eM := parseTimeToMinutes(eStr)
		if sM < 0 || sM >= 1440 || eM < 0 || eM >= 1440 {
			return nil, fmt.Errorf("时间格式不正确 (需为 HH:MM)")
		}
		if sM == eM {
			return nil, fmt.Errorf("开始时间与结束时间不能相同 (%s)", sStr)
		}
		if sM < eM {
			return []interval{{start: sM, end: eM}}, nil
		}
		// Cross midnight
		return []interval{{start: sM, end: 1440}, {start: 0, end: eM}}, nil
	}

	dayNames := map[int]string{1: "周一", 2: "周二", 3: "周三", 4: "周四", 5: "周五", 6: "周六", 7: "周日"}

	for i := 0; i < len(slots); i++ {
		s1 := slots[i]
		int1, err := getIntervals(s1.Start, s1.End)
		if err != nil {
			name := s1.Name
			if name == "" {
				name = fmt.Sprintf("时段 %d", i+1)
			}
			return fmt.Errorf("[%s] %w", name, err)
		}
		days1 := s1.Days
		if len(days1) == 0 {
			days1 = []int{1, 2, 3, 4, 5, 6, 7}
		}

		for j := i + 1; j < len(slots); j++ {
			s2 := slots[j]
			int2, err := getIntervals(s2.Start, s2.End)
			if err != nil {
				name := s2.Name
				if name == "" {
					name = fmt.Sprintf("时段 %d", j+1)
				}
				return fmt.Errorf("[%s] %w", name, err)
			}
			days2 := s2.Days
			if len(days2) == 0 {
				days2 = []int{1, 2, 3, 4, 5, 6, 7}
			}

			// Check common days
			var commonDays []int
			for _, d1 := range days1 {
				for _, d2 := range days2 {
					if d1 == d2 {
						commonDays = append(commonDays, d1)
						break
					}
				}
			}
			if len(commonDays) == 0 {
				continue
			}

			// Check interval overlap
			overlap := false
			for _, r1 := range int1 {
				for _, r2 := range int2 {
					maxS := r1.start
					if r2.start > maxS {
						maxS = r2.start
					}
					minE := r1.end
					if r2.end < minE {
						minE = r2.end
					}
					if maxS < minE {
						overlap = true
						break
					}
				}
				if overlap {
					break
				}
			}

			if overlap {
				var dayStrs []string
				for _, d := range commonDays {
					dayStrs = append(dayStrs, dayNames[d])
				}
				name1 := s1.Name
				if name1 == "" {
					name1 = fmt.Sprintf("时段 %d", i+1)
				}
				name2 := s2.Name
				if name2 == "" {
					name2 = fmt.Sprintf("时段 %d", j+1)
				}
				return fmt.Errorf("[%s] (%s-%s) 与 [%s] (%s-%s) 在 %s 存在时间重叠，请调整时段",
					name1, s1.Start, s1.End, name2, s2.Start, s2.End, strings.Join(dayStrs, "、"))
			}
		}
	}

	return nil
}

// IsOffPeakDefault evaluates whether current server time matches DeepSeek official off-peak hours.
func IsOffPeakDefault(t time.Time) bool {
	localT := t.In(shanghaiLoc)
	weekday := localT.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return true
	}
	m := localT.Hour()*60 + localT.Minute()
	if (m >= 540 && m < 720) || (m >= 840 && m < 1080) {
		return false
	}
	return true
}

func parseTimeToMinutes(s string) int {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}

// CalculateCostDetailedWithGroup computes total request cost, total saved amount, cache savings, and off-peak discount metrics for a specified pricing group.
func (e *BillingEngine) CalculateCostDetailedWithGroup(modelName, groupName string, promptTokens, completionTokens, cachedTokens int, t time.Time) (cost float64, totalSaved float64, cacheSaved float64, isOffPeak bool, discount float64) {
	if e == nil {
		return 0, 0, 0, false, 1.0
	}

	p := e.GetPriceWithGroup(modelName, groupName)
	if p == nil {
		return 0, 0, 0, false, 1.0
	}

	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if cachedTokens > promptTokens {
		cachedTokens = promptTokens
	}

	uncachedPrompt := promptTokens - cachedTokens

	// Standard benchmark token costs
	promptCost := (float64(uncachedPrompt) * p.PromptPrice) / 1_000_000.0
	cachedCost := (float64(cachedTokens) * p.CacheReadPrice) / 1_000_000.0
	compCost := (float64(completionTokens) * p.CompletionPrice) / 1_000_000.0
	tokenCost := promptCost + cachedCost + compCost

	if cachedTokens > 0 && p.PromptPrice > p.CacheReadPrice {
		cacheSaved = (float64(cachedTokens) * (p.PromptPrice - p.CacheReadPrice)) / 1_000_000.0
	}

	// Time-of-Use Off-Peak Discount Evaluation
	isOffPeak, discount = e.IsOffPeak(p, t)
	if isOffPeak {
		discountedTokenCost := tokenCost * discount
		offPeakSaved := tokenCost - discountedTokenCost
		cost = discountedTokenCost + p.FixedPrice
		totalSaved = cacheSaved + offPeakSaved
	} else {
		cost = tokenCost + p.FixedPrice
		totalSaved = cacheSaved
		discount = 1.0
	}

	cost = roundPrecision(cost)
	totalSaved = roundPrecision(totalSaved)
	cacheSaved = roundPrecision(cacheSaved)

	return cost, totalSaved, cacheSaved, isOffPeak, discount
}

// roundPrecision rounds monetary amounts to 8 decimal places (fraction of micro-cents)
// to eliminate IEEE-754 binary floating point precision drift.
func roundPrecision(val float64) float64 {
	return math.Round(val*1e8) / 1e8
}

// CalculateCostDetailed computes total request cost, total saved amount, cache savings, and off-peak discount metrics using the default group.
func (e *BillingEngine) CalculateCostDetailed(modelName string, promptTokens, completionTokens, cachedTokens int, t time.Time) (cost float64, totalSaved float64, cacheSaved float64, isOffPeak bool, discount float64) {
	return e.CalculateCostDetailedWithGroup(modelName, "default", promptTokens, completionTokens, cachedTokens, t)
}

// CalculateCostWithGroup computes total request cost and savings using current server time and the specified pricing group.
func (e *BillingEngine) CalculateCostWithGroup(modelName, groupName string, promptTokens, completionTokens, cachedTokens int) (cost float64, savedCost float64) {
	cost, totalSaved, _, _, _ := e.CalculateCostDetailedWithGroup(modelName, groupName, promptTokens, completionTokens, cachedTokens, time.Now())
	return cost, totalSaved
}

// CalculateCost computes total request cost and savings using current server time and default group.
func (e *BillingEngine) CalculateCost(modelName string, promptTokens, completionTokens, cachedTokens int) (cost float64, savedCost float64) {
	return e.CalculateCostWithGroup(modelName, "default", promptTokens, completionTokens, cachedTokens)
}

