package budget

import (
	"fmt"
	"time"
)

// Period is the reset cadence of a budget. The gateway uses the operator's
// business timezone (Asia/Shanghai) so a "daily" budget resets at local
// midnight, not at the UTC boundary.
type Period string

const (
	PeriodDaily   Period = "daily"
	PeriodMonthly Period = "monthly"
	PeriodTotal   Period = "total"
)

var shanghai = time.FixedZone("CST", 8*3600)

// Valid reports whether p is one of the supported periods.
func (p Period) Valid() bool {
	switch p {
	case PeriodDaily, PeriodMonthly, PeriodTotal:
		return true
	}
	return false
}

// ParsePeriod normalizes operator input.
func ParsePeriod(raw string) (Period, error) {
	p := Period(raw)
	if !p.Valid() {
		return "", fmt.Errorf("invalid budget period %q (want daily, monthly or total)", raw)
	}
	return p, nil
}

// Key is the stable identifier of the period containing at. It is used as the
// budget_event dedupe key and as the Redis counter suffix, so changing it would
// re-notify every threshold.
func (p Period) Key(at time.Time) string {
	local := at.In(shanghai)
	switch p {
	case PeriodDaily:
		return local.Format("2006-01-02")
	case PeriodMonthly:
		return local.Format("2006-01")
	case PeriodTotal:
		return "total"
	}
	return ""
}

// Start is the first instant of the period containing at. A total budget has no
// start (zero time) because it never resets.
func (p Period) Start(at time.Time) time.Time {
	local := at.In(shanghai)
	switch p {
	case PeriodDaily:
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghai)
	case PeriodMonthly:
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, shanghai)
	}
	return time.Time{}
}
