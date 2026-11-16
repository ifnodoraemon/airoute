package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func checkOverviewSpendWindows(t *testing.T, db *DB) {
	t.Helper()
	r := NewRepository(db)
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	owner := HashAPIKey("window-owner")
	for i, fixture := range []struct {
		age   time.Duration
		units int64
		key   string
	}{
		{time.Hour, 1234567891, owner},
		{18 * time.Hour, 2000000000, owner},
		{36 * time.Hour, 3000000000, owner},
		{10 * 24 * time.Hour, 4000000000, owner},
		{40 * 24 * time.Hour, 5000000000, owner},
		{time.Hour, 99000000000, HashAPIKey("other-tenant")},
	} {
		_, err := db.Exec(`INSERT INTO usage_logs (trace_id, api_key, model, cost_units, created_at, billing_id) VALUES (?, ?, 'm', ?, ?, ?)`,
			"spend-window", fixture.key, fixture.units, r.dialectTimePrecise(now.Add(-fixture.age)), "spend-window-"+string(rune('a'+i)))
		require.NoError(t, err)
	}
	stats := &StatsOverview{}
	require.NoError(t, r.loadOverviewSpend(stats, []string{owner}, now))
	require.Equal(t, 1.234567891, stats.TodayCost)
	require.Equal(t, 10.234567891, stats.MonthCost)
	require.Equal(t, 3.234567891, stats.Cost24H)
	require.Equal(t, 3.0, stats.PreviousCost24H)
	require.NoError(t, r.loadOverviewSpend(stats, []string{HashAPIKey("empty-owner")}, now))
	require.Zero(t, stats.MonthCost, "empty tenant scope must not reveal cluster spend")
}

func TestOverviewSpendCalendarAndTenantScope(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	checkOverviewSpendWindows(t, db)
}
