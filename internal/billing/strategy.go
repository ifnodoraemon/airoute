package billing

import (
	"encoding/json"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// OffPeakSlot represents a discrete discount time window.
//
// Days names the ISO weekday the window STARTS on (1=Mon … 7=Sun; empty = every
// day). With start-day semantics a cross-midnight slot such as 22:00-06:00 with
// Days=[1] covers Monday 22:00 → Tuesday 06:00: the after-midnight half belongs
// to the window that began the previous evening, not to a second window on the
// configured day. The write-path overlap validation uses the same rule (see
// slotSegments in strategy_validation.go), so validation and pricing cannot
// disagree about which day a slot's tail belongs to.
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
	startM, endM, ok := parseRowWindow(p.OffPeakStart, p.OffPeakEnd)
	if !ok {
		// A malformed row-level window never invents a discount: charging the
		// undiscounted price is the only safe reading of "00:0x".
		return false, 1.0
	}
	if startM == 0 && endM == 0 {
		// Unset (or "00:00"-"00:00") keeps the historical night default 00:00-08:30.
		startM = 0
		endM = 510 // 08:30
	}
	if clockInWindow(ctx.CurrentM, startM, endM) {
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
		sM, startErr := parseClockMinutes(slot.Start)
		eM, endErr := parseClockMinutes(slot.End)
		if startErr != nil || endErr != nil {
			// Rows can predate strict validation. A slot with an unparsable time can
			// never be honoured as written, so it is skipped instead of being
			// reinterpreted as some other window (the old lenient parser turned
			// "8:30pm" into 08:00 and "abc" into midnight).
			telemetry.WarnLogger(telemetry.ScopedLogger("billing"),
				"offpeak-slot-invalid-time:"+p.Model+":"+slot.Name+":"+slot.Start+"-"+slot.End,
				60*time.Second,
				"off-peak slot has a malformed time; the slot is ignored and the undiscounted price applies",
				"model", p.Model, "slot", slot.Name, "start", slot.Start, "end", slot.End)
			continue
		}

		d := slot.Discount
		if d <= 0 || d >= 1.0 {
			// Same rule as the row-level discount: an invalid coefficient means "no
			// discount", never "use the row's coefficient". Falling back to
			// ctx.Discount turned a typo into a 50% discount - an operator writing
			// 1.5 (intending a surcharge) or 0 (intending no discount) silently billed
			// half price. Clamping to 1.0 keeps "never invent a discount" true at both
			// levels; ValidateOffPeakSlots rejects such slots on write, so this is the
			// defence for rows that predate that validation.
			d = 1.0
			telemetry.WarnLogger(telemetry.ScopedLogger("billing"),
				"offpeak-slot-invalid-discount:"+p.Model+":"+slot.Name+":"+slot.Start+"-"+slot.End,
				60*time.Second,
				"off-peak slot has an invalid discount; charging the undiscounted price",
				"model", p.Model, "slot", slot.Name, "start", slot.Start, "end", slot.End,
				"discount", slot.Discount)
		}

		if !slotCoversInstant(slot.Days, sM, eM, ctx.ISODay, ctx.CurrentM) {
			continue
		}
		matched = true
		if d < bestDiscount {
			bestDiscount = d
		}
	}

	// 4. If no custom multi-slots configured, evaluate default start/end window
	if len(slots) == 0 && (p.OffPeakStart != "" || p.OffPeakEnd != "") {
		startM, endM, ok := parseRowWindow(p.OffPeakStart, p.OffPeakEnd)
		if ok {
			if startM == 0 && endM == 0 {
				startM = 0
				endM = 510 // 08:30
			}
			if clockInWindow(ctx.CurrentM, startM, endM) {
				matched = true
				if ctx.Discount < bestDiscount {
					bestDiscount = ctx.Discount
				}
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
	startM, endM, ok := parseRowWindow(p.OffPeakStart, p.OffPeakEnd)
	if !ok {
		return false, 1.0
	}
	if clockInWindow(ctx.CurrentM, startM, endM) {
		return true, ctx.Discount
	}
	return false, 1.0
}

var offPeakStrategies = map[string]OffPeakStrategy{
	"night":  &NightOffPeakStrategy{},
	"custom": &CustomOffPeakStrategy{},
	"none":   &NoneOffPeakStrategy{},
}

// clockInWindow reports whether a minute-of-day lies inside [startM, endM). A
// window whose start is at or after its end wraps around midnight.
func clockInWindow(currentM, startM, endM int) bool {
	if startM < endM {
		return currentM >= startM && currentM < endM
	}
	return currentM >= startM || currentM < endM
}

// slotCoversInstant reports whether a slot covers the given instant, using
// start-day semantics for the day filter.
//
//	slot 22:00-06:00 days=[1]  →  Monday 22:00-23:59 and Tuesday 00:00-05:59
//
// The after-midnight half is matched against the *previous* ISO weekday, because
// that is the day the window started on; testing it against the current weekday
// (the previous behaviour) discounted the wrong night: every configured night lost
// its 6-hour tail and a second, unintended 6-hour window on the configured day was
// discounted instead.
func slotCoversInstant(days []int, startM, endM, isoDay, currentM int) bool {
	if startM < endM {
		return clockInWindow(currentM, startM, endM) && offPeakDayMatches(days, isoDay)
	}
	// Cross-midnight: the evening half belongs to today, the morning half to the
	// window that started yesterday.
	if currentM >= startM {
		return offPeakDayMatches(days, isoDay)
	}
	if currentM < endM {
		return offPeakDayMatches(days, previousISODay(isoDay))
	}
	return false
}

// offPeakDayMatches reports whether the ISO weekday (1=Mon … 7=Sun) is covered by
// the slot's day list. An empty list means every day.
func offPeakDayMatches(days []int, isoDay int) bool {
	if len(days) == 0 {
		return true
	}
	for _, day := range days {
		if day == isoDay {
			return true
		}
	}
	return false
}

// previousISODay returns the ISO weekday before isoDay; Monday's previous is Sunday.
func previousISODay(isoDay int) int {
	if isoDay <= 1 {
		return 7
	}
	return isoDay - 1
}
