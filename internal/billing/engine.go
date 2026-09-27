package billing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

// GlobalEngine is the singleton billing calculator used by all data plane handlers.
var GlobalEngine *BillingEngine

// OffPeakSlot represents a discrete discount time window.
type OffPeakSlot struct {
	Start    string  `json:"start"`          // e.g. "00:00"
	End      string  `json:"end"`            // e.g. "08:30"
	Discount float64 `json:"discount"`       // e.g. 0.5
	Name     string  `json:"name,omitempty"` // e.g. "夜间优惠"
	Days     []int   `json:"days,omitempty"` // 1=Mon, 2=Tue, 3=Wed, 4=Thu, 5=Fri, 6=Sat, 7=Sun (empty = all days)
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
			OffPeakMode:     "deepseek",
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
		m[strings.ToLower(rec.Model)] = rec
	}

	e.mu.Lock()
	e.prices = m
	e.mu.Unlock()

	telemetry.Logger.Info("reloaded model pricing engine", "rules_count", len(list))
	return nil
}

// GetPrice finds the applicable pricing rate for a requested model.
func (e *BillingEngine) GetPrice(modelName string) *storage.ModelPriceRecord {
	modelLower := strings.ToLower(strings.TrimSpace(modelName))

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Exact match
	if p, ok := e.prices[modelLower]; ok {
		return p
	}

	// 2. Wildcard prefix match (e.g. "gpt-4o-*" matching "gpt-4o-2024-08-06")
	for pattern, p := range e.prices {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(modelLower, prefix) {
				return p
			}
		}
	}

	// 3. Fallback wildcard rule "*"
	if p, ok := e.prices["*"]; ok {
		return p
	}

	// 4. Default standard fallback
	return e.deflt
}

// IsOffPeak evaluates whether the specified time is within the model's off-peak time window.
// Supports:
// 1. "deepseek" (DeepSeek official rule: Mon-Fri 09:00-12:00, 14:00-18:00 Peak, all other times and weekends 50% discount)
// 2. "night" (00:00 - 08:30 discount)
// 3. "custom" (multiple customizable slots + weekend all-day toggle)
// 4. "none" (no discount)
func (e *BillingEngine) IsOffPeak(p *storage.ModelPriceRecord, t time.Time) (bool, float64) {
	if p == nil || !p.OffPeakEnabled {
		return false, 1.0
	}

	discount := p.OffPeakDiscount
	if discount <= 0 || discount >= 1.0 {
		discount = 0.5
	}

	// China Standard Time (CST / UTC+8) evaluation
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	localT := t.In(loc)
	weekday := localT.Weekday()
	isWeekend := (weekday == time.Saturday || weekday == time.Sunday)
	isoDay := int(weekday)
	if isoDay == 0 {
		isoDay = 7 // Sunday = 7
	}
	currentM := localT.Hour()*60 + localT.Minute()

	mode := strings.ToLower(strings.TrimSpace(p.OffPeakMode))
	if mode == "" {
		mode = "deepseek"
	}

	switch mode {
	case "deepseek":
		// DeepSeek Official Schedule:
		// Peak: Mon-Fri 09:00 - 12:00 & 14:00 - 18:00
		// Off-Peak: Weekends all day, plus Mon-Fri 18:00-09:00 (nights) & 12:00-14:00 (noon)
		if isWeekend {
			return true, discount
		}
		// Check peak 1 (09:00 - 12:00) and peak 2 (14:00 - 18:00)
		if (currentM >= 540 && currentM < 720) || (currentM >= 840 && currentM < 1080) {
			return false, 1.0 // Peak
		}
		return true, discount // Off-peak (night or noon)

	case "night":
		startM := parseTimeToMinutes(p.OffPeakStart)
		endM := parseTimeToMinutes(p.OffPeakEnd)
		if startM == 0 && endM == 0 {
			startM = 0
			endM = 510 // 08:30
		}
		var isOff bool
		if startM <= endM {
			isOff = currentM >= startM && currentM < endM
		} else {
			isOff = currentM >= startM || currentM < endM
		}
		if isOff {
			return true, discount
		}
		return false, 1.0

	case "custom":
		bestDiscount := 1.0
		matched := false

		// 1. Weekend all-day evaluation if enabled
		if p.WeekendAllDay && isWeekend {
			matched = true
			bestDiscount = discount
		}

		// 2. Parse slots
		var slots []OffPeakSlot
		if p.OffPeakSlots != "" {
			_ = json.Unmarshal([]byte(p.OffPeakSlots), &slots)
		}

		// 3. Evaluate each slot with conflict resolution: Best Discount Rule (lowest discount multiplier)
		for _, slot := range slots {
			// Day of week match: empty means all days (1-7)
			if len(slot.Days) > 0 {
				dayMatch := false
				for _, d := range slot.Days {
					if d == isoDay {
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
				d = discount
			}

			var inSlot bool
			if sM <= eM {
				inSlot = currentM >= sM && currentM < eM
			} else {
				// Cross midnight, e.g. 22:00 -> 06:00
				inSlot = currentM >= sM || currentM < eM
			}

			if inSlot {
				matched = true
				if d < bestDiscount {
					bestDiscount = d
				}
			}
		}

		if matched {
			return true, bestDiscount
		}
		return false, 1.0

	case "none":
		return false, 1.0

	default:
		// Fallback to start/end window
		startM := parseTimeToMinutes(p.OffPeakStart)
		endM := parseTimeToMinutes(p.OffPeakEnd)
		var isOff bool
		if startM <= endM {
			isOff = currentM >= startM && currentM < endM
		} else {
			isOff = currentM >= startM || currentM < endM
		}
		if isOff {
			return true, discount
		}
		return false, 1.0
	}
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
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	localT := t.In(loc)
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

// CalculateCostDetailed computes total request cost, total saved amount, cache savings, and off-peak discount metrics.
func (e *BillingEngine) CalculateCostDetailed(modelName string, promptTokens, completionTokens, cachedTokens int, t time.Time) (cost float64, totalSaved float64, cacheSaved float64, isOffPeak bool, discount float64) {
	if e == nil {
		return 0, 0, 0, false, 1.0
	}

	p := e.GetPrice(modelName)
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

	return cost, totalSaved, cacheSaved, isOffPeak, discount
}

// CalculateCost computes total request cost and savings using current server time.
func (e *BillingEngine) CalculateCost(modelName string, promptTokens, completionTokens, cachedTokens int) (cost float64, savedCost float64) {
	cost, totalSaved, _, _, _ := e.CalculateCostDetailed(modelName, promptTokens, completionTokens, cachedTokens, time.Now())
	return cost, totalSaved
}
