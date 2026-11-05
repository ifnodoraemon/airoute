//go:build integration

package storage

import (
	"context"
	"testing"
	"time"
)

func TestPostgresChannelPathRepairKeepsAuditAmounts(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	name := "identity-" + itSuffix()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM usage_logs WHERE model=?`, name) })
	rec := &UsageLogRecord{TraceID: name, Model: name, Channel: "primary→backup", StatusCode: 200, Cost: 0.000000019, TotalTokens: 17, CreatedAt: time.Now()}
	if err := repo.RecordUsageLogContext(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	// A v51 replica or old queue delivery can still carry the combined identity.
	if _, err := db.Exec(`UPDATE usage_logs SET channel='primary→backup',channel_trail='' WHERE billing_id=?`, rec.BillingID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := separateChannelTrails(db); err != nil {
			t.Fatal(err)
		}
	}
	var channel, trail string
	var amount, tokens int64
	if err := db.QueryRow(`SELECT channel,channel_trail,cost_units,total_tokens FROM usage_logs WHERE billing_id=?`, rec.BillingID).Scan(&channel, &trail, &amount, &tokens); err != nil {
		t.Fatal(err)
	}
	if channel != "backup" || trail != "primary→backup" || amount != 19 || tokens != 17 {
		t.Fatalf("repair=%q %q %d %d", channel, trail, amount, tokens)
	}
	logs, err := repo.ListUsageLogsWithFilter(LogFilter{Model: name, Keyword: "primary"})
	if err != nil || len(logs) != 1 || logs[0].Channel != "backup" {
		t.Fatalf("path search=%+v err=%v", logs, err)
	}
	report, err := repo.GetSupplierReconciliation(ChannelReconciliationFilter{Model: name, Channel: "backup"})
	if err != nil || report.Summary.TotalRequests != 1 {
		t.Fatalf("supplier report=%+v err=%v", report, err)
	}
}
