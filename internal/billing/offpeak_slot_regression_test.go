package billing

import (
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// crossMidnightTestEngine returns an engine with one custom cross-midnight slot:
// 22:00-06:00 (see the day list in each test).
func crossMidnightTestEngine(slots string) *BillingEngine {
	return &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{
			"default:slot-model": {
				Model:           "slot-model",
				GroupName:       "default",
				OffPeakEnabled:  true,
				OffPeakMode:     "custom",
				OffPeakDiscount: 0.5,
				OffPeakSlots:    slots,
			},
		},
	}
}

// BUG-DATA-02 regression: a cross-midnight off-peak slot uses start-day semantics.
// days=[1] on 22:00-06:00 means "Monday 22:00 → Tuesday 06:00", so the tail is
// discounted on Tuesday morning and the Monday pre-dawn hours are not.
func TestCrossMidnightSlotUsesStartDay(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	engine := crossMidnightTestEngine(`[{"start":"22:00","end":"06:00","discount":0.5,"days":[1],"name":"monday-night"}]`)

	// 2026-09-21 is a Monday; 2026-09-22 is the Tuesday that follows it.
	cases := []struct {
		name     string
		at       time.Time
		wantOff  bool
		wantDisc float64
	}{
		{"monday evening starts the window", time.Date(2026, 9, 21, 23, 0, 0, 0, loc), true, 0.5},
		{"tuesday after midnight is the window's tail", time.Date(2026, 9, 22, 1, 0, 0, 0, loc), true, 0.5},
		{"tuesday 06:00 is the exclusive end", time.Date(2026, 9, 22, 6, 0, 0, 0, loc), false, 1.0},
		{"tuesday evening is a different night", time.Date(2026, 9, 22, 22, 30, 0, 0, loc), false, 1.0},
		{"monday pre-dawn belongs to sunday night", time.Date(2026, 9, 21, 1, 0, 0, 0, loc), false, 1.0},
		{"wednesday pre-dawn is not this window", time.Date(2026, 9, 23, 1, 0, 0, 0, loc), false, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			off, discount := engine.IsOffPeak(engine.GetPriceWithGroup("slot-model", "default"), tc.at)
			if off != tc.wantOff || discount != tc.wantDisc {
				t.Fatalf("%s: isOff=%v discount=%v, want %v/%v", tc.name, off, discount, tc.wantOff, tc.wantDisc)
			}
		})
	}
}

// An empty day list still means "every day", including the cross-midnight tail.
func TestCrossMidnightSlotWithoutDaysMatchesEveryNight(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	engine := crossMidnightTestEngine(`[{"start":"22:00","end":"06:00","discount":0.4}]`)
	price := engine.GetPriceWithGroup("slot-model", "default")

	for _, at := range []time.Time{
		time.Date(2026, 9, 21, 23, 0, 0, 0, loc),  // Monday evening
		time.Date(2026, 9, 22, 4, 30, 0, 0, loc),  // Tuesday pre-dawn
		time.Date(2026, 9, 27, 1, 15, 0, 0, loc),  // Sunday pre-dawn
		time.Date(2026, 9, 27, 23, 59, 0, 0, loc), // Sunday evening
	} {
		if off, discount := engine.IsOffPeak(price, at); !off || discount != 0.4 {
			t.Fatalf("%s: isOff=%v discount=%v, want true/0.4", at, off, discount)
		}
	}
	// Outside the window, every day.
	for _, at := range []time.Time{
		time.Date(2026, 9, 22, 6, 0, 0, 0, loc),
		time.Date(2026, 9, 22, 12, 0, 0, 0, loc),
	} {
		if off, discount := engine.IsOffPeak(price, at); off || discount != 1.0 {
			t.Fatalf("%s: isOff=%v discount=%v, want false/1.0", at, off, discount)
		}
	}
}

// A non-cross-midnight slot keeps matching the current weekday only.
func TestNonCrossMidnightSlotDayFilterRegression(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	engine := crossMidnightTestEngine(`[{"start":"08:00","end":"18:00","discount":0.6,"days":[3]}]`)
	price := engine.GetPriceWithGroup("slot-model", "default")

	if off, discount := engine.IsOffPeak(price, time.Date(2026, 9, 23, 10, 0, 0, 0, loc)); !off || discount != 0.6 {
		t.Fatalf("Wednesday 10:00: isOff=%v discount=%v, want true/0.6", off, discount)
	}
	if off, _ := engine.IsOffPeak(price, time.Date(2026, 9, 24, 10, 0, 0, 0, loc)); off {
		t.Fatal("Thursday 10:00 matched a Wednesday-only slot")
	}
	if off, _ := engine.IsOffPeak(price, time.Date(2026, 9, 23, 19, 0, 0, 0, loc)); off {
		t.Fatal("Wednesday 19:00 matched an 08:00-18:00 slot")
	}
}

// BUG-DATA-02 validation half: the overlap check attributes a cross-midnight
// tail to the *next* day, so it catches overlaps that span midnight and does not
// invent overlaps that cannot happen.
func TestValidateSlotsCrossMidnightOverlapUsesStartDay(t *testing.T) {
	// Monday-night window (22:00-06:00, Mon) overlaps a Tuesday-morning slot:
	// the tail is Tuesday 00:00-06:00.
	crossDayOverlap := `[
		{"name":"周一夜间","start":"22:00","end":"06:00","discount":0.5,"days":[1]},
		{"name":"周二早间","start":"05:00","end":"09:00","discount":0.5,"days":[2]}
	]`
	if err := ValidateOffPeakSlots("custom", crossDayOverlap); err == nil {
		t.Fatal("expected cross-day overlap (Tuesday 05:00-06:00) to be rejected")
	} else if !strings.Contains(err.Error(), "周二") {
		t.Fatalf("overlap error should name the effective day (周二), got %v", err)
	}

	// Same shape but the morning slot is on Wednesday: no overlap.
	noOverlap := `[
		{"name":"周一夜间","start":"22:00","end":"06:00","discount":0.5,"days":[1]},
		{"name":"周三早间","start":"05:00","end":"09:00","discount":0.5,"days":[3]}
	]`
	if err := ValidateOffPeakSlots("custom", noOverlap); err != nil {
		t.Fatalf("expected no overlap, got %v", err)
	}

	// Two consecutive nights' windows (Mon and Tue, both 22:00-06:00) do not
	// overlap: Monday's tail ends before Tuesday's 22:00 start.
	consecutiveNights := `[
		{"name":"周一夜间","start":"22:00","end":"06:00","discount":0.5,"days":[1]},
		{"name":"周二夜间","start":"22:00","end":"06:00","discount":0.5,"days":[2]}
	]`
	if err := ValidateOffPeakSlots("custom", consecutiveNights); err != nil {
		t.Fatalf("consecutive nights must not be reported as overlapping, got %v", err)
	}

	// Nightly window plus an early-morning window on every day still overlaps.
	allDays := `[
		{"name":"夜间","start":"22:00","end":"06:00","discount":0.5},
		{"name":"早间","start":"05:00","end":"09:00","discount":0.5}
	]`
	if err := ValidateOffPeakSlots("custom", allDays); err == nil {
		t.Fatal("expected nightly window and early-morning window to overlap")
	}
}

// BUG-DATA-03 regression: times are parsed strictly, so malformed values fail the
// write path instead of silently becoming a different window.
func TestParseClockMinutesStrict(t *testing.T) {
	valid := map[string]int{
		"00:00":    0,
		"0:00":     0,
		"08:30":    510,
		"8:30":     510,
		"23:59":    1439,
		" 22:00  ": 1320,
	}
	for raw, want := range valid {
		got, err := parseClockMinutes(raw)
		if err != nil || got != want {
			t.Fatalf("parseClockMinutes(%q) = %d, %v; want %d, nil", raw, got, err, want)
		}
	}

	invalid := []string{
		"",        // missing
		"abc",     // not a clock
		"8:30pm",  // AM/PM is not accepted
		"8",       // missing minutes
		"8:5",     // minutes must be two digits
		"123:00",  // three-digit hour
		"24:00",   // hour out of range
		"23:60",   // minute out of range
		"-1:00",   // negative hour
		"+8:30",   // signed hour
		"08:30:0", // extra field
	}
	for _, raw := range invalid {
		if got, err := parseClockMinutes(raw); err == nil {
			t.Fatalf("parseClockMinutes(%q) = %d, nil; want an error", raw, got)
		}
	}
}

func TestValidateSlotsRejectsMalformedTimes(t *testing.T) {
	cases := map[string]string{
		"am/pm suffix":     `[{"start":"8:30pm","end":"21:30","discount":0.5}]`,
		"non-numeric":      `[{"start":"abc","end":"06:00","discount":0.5}]`,
		"hour too big":     `[{"start":"00:00","end":"25:00","discount":0.5}]`,
		"minute too big":   `[{"start":"00:00","end":"08:60","discount":0.5}]`,
		"one digit minute": `[{"start":"00:00","end":"08:5","discount":0.5}]`,
	}
	for name, slots := range cases {
		if err := ValidateOffPeakSlots("custom", slots); err == nil {
			t.Errorf("%s: expected a validation error, got nil", name)
		}
	}
	// Strict formats that are legitimate stay accepted.
	if err := ValidateOffPeakSlots("custom", `[{"start":"0:00","end":"23:59","discount":0.5,"days":[1]}]`); err != nil {
		t.Fatalf("valid 0:00-23:59 slot rejected: %v", err)
	}
}

// A malformed row-level window (legacy row or an import) must charge the
// undiscounted price rather than reinterpret the typo as a window.
func TestMalformedRowWindowChargesUndiscounted(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, loc)

	for _, mode := range []string{"night", "custom", ""} {
		engine := &BillingEngine{
			prices: map[string]*storage.ModelPriceRecord{
				"default:bad-window": {
					Model:           "bad-window",
					GroupName:       "default",
					OffPeakEnabled:  true,
					OffPeakMode:     mode,
					OffPeakStart:    "8:30pm",
					OffPeakEnd:      "23:00",
					OffPeakDiscount: 0.5,
				},
			},
		}
		if off, discount := engine.IsOffPeak(engine.GetPriceWithGroup("bad-window", "default"), at); off || discount != 1.0 {
			t.Fatalf("mode %q: malformed row window gave isOff=%v discount=%v, want false/1.0", mode, off, discount)
		}
	}
}

// The historical night default (00:00-08:30 when no window is configured) is
// preserved: it is a deliberate default, not a lenient-parse artefact.
func TestNightModeKeepsDefaultWindow(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	engine := &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{
			"default:night-default": {
				Model:           "night-default",
				GroupName:       "default",
				OffPeakEnabled:  true,
				OffPeakMode:     "night",
				OffPeakDiscount: 0.5,
			},
		},
	}
	price := engine.GetPriceWithGroup("night-default", "default")
	if off, discount := engine.IsOffPeak(price, time.Date(2026, 10, 27, 3, 0, 0, 0, loc)); !off || discount != 0.5 {
		t.Fatalf("night default 03:00: isOff=%v discount=%v, want true/0.5", off, discount)
	}
	if off, _ := engine.IsOffPeak(price, time.Date(2026, 10, 27, 9, 0, 0, 0, loc)); off {
		t.Fatal("night default 09:00 matched, want peak")
	}
}
