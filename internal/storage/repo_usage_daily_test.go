package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/budget"
)

func seedChannelAndUsage(t *testing.T, db *DB) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO channels (name, type, base_url, api_key, models, status)
		VALUES ('ch-a', 'openai', 'http://example.invalid', 'k', 'm-1', 'active')`)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	channelID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO usage_logs
		(api_key, tenant_id, org_id, model, channel, channel_id, prompt_tokens, completion_tokens, total_tokens, cost, cost_units, cost_basis_units, status_code, created_at)
		VALUES ('k', 't', 1, 'm-1', 'ch-a', ?, 100, 50, 150, 3, 3000000000, 2000000000, 200, '2026-10-27 10:00:00')`, channelID); err != nil {
		t.Fatalf("insert usage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO usage_logs
		(api_key, tenant_id, org_id, model, channel, channel_id, prompt_tokens, completion_tokens, total_tokens, cost, cost_units, status_code, created_at)
		VALUES ('k', 't', 1, 'm-1', 'ch-a', ?, 10, 0, 10, 1, 1000000000, 200, '2026-10-27 11:00:00')`, channelID); err != nil {
		t.Fatalf("insert usage 2: %v", err)
	}
	// Next day must not leak into the bucket.
	if _, err := db.Exec(`INSERT INTO usage_logs (api_key, tenant_id, org_id, model, channel, created_at)
		VALUES ('k', 't', 1, 'm-1', 'ch-a', '2026-10-28 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	return channelID
}

func TestRefreshUsageDailyAndTrend(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	channelID := seedChannelAndUsage(t, db)

	if err := repo.RefreshUsageDaily(context.Background(), "2026-10-27"); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListUsageDaily("2026-10-27", "2026-10-27")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	row := rows[0]
	if row.OrgID != 1 || row.ChannelID != channelID || row.Model != "m-1" {
		t.Fatalf("group = %+v", row)
	}
	if row.Requests != 2 || row.TotalTokens != 160 || row.BilledUnits != ToUnits(4) || row.CostBasisUnits != ToUnits(2) || row.KnownCostRequests != 1 {
		t.Fatalf("aggregate = %+v", row)
	}
	// Replay must not double count.
	if err := repo.RefreshUsageDaily(context.Background(), "2026-10-27"); err != nil {
		t.Fatal(err)
	}
	rows, _ = repo.ListUsageDaily("2026-10-27", "2026-10-27")
	if len(rows) != 1 || rows[0].Requests != 2 {
		t.Fatalf("replay rows = %+v", rows)
	}
	trend, err := repo.CostTrend("2026-10-27", "2026-10-27")
	if err != nil || len(trend) != 1 || trend[0].BilledUnits != ToUnits(4) {
		t.Fatalf("trend = %+v err=%v", trend, err)
	}
	if err := repo.EnsureUsageDaily(context.Background(), "2026-10-27", "2026-10-28"); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillUsageLogChannelIDs(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	res, err := db.Exec(`INSERT INTO channels (name, type, base_url, api_key, models, status)
		VALUES ('ch-b', 'openai', 'http://example.invalid', 'k', 'm', 'active')`)
	if err != nil {
		t.Fatal(err)
	}
	channelID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO usage_logs (api_key, tenant_id, model, channel, created_at)
		VALUES ('k', 't', 'm', 'ch-b', '2026-10-27 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if err := backfillUsageLogChannelIDs(db); err != nil {
		t.Fatal(err)
	}
	var got *int64
	if err := db.QueryRow(`SELECT channel_id FROM usage_logs WHERE channel = 'ch-b'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != channelID {
		t.Fatalf("channel_id = %v, want %d", got, channelID)
	}
}

func TestReconcileBudgetsFromUsageLogs(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	rec := &BudgetRecord{ScopeType: "org", ScopeID: 1, Period: string(budget.PeriodMonthly), AmountUnits: ToUnits(1000)}
	if err := repo.UpsertBudget(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO usage_logs (api_key, tenant_id, org_id, model, channel, cost_units, total_tokens, created_at)
		VALUES ('k', 't', 1, 'm', 'ch', ?, 10, ?)`, ToUnits(2.5), time.Now().UTC().Format(sqliteTimestampLayout)); err != nil {
		t.Fatal(err)
	}
	count, err := repo.ReconcileBudgets(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled = %d", count)
	}
	stored, err := repo.GetBudget(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UsedUnits != ToUnits(2.5) {
		t.Fatalf("used_units = %d, want %d", stored.UsedUnits, ToUnits(2.5))
	}
}
