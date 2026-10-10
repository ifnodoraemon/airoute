package billing

import (
	"encoding/json"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

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
