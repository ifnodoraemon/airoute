package storage

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestChannelIdentityMigrationPreservesBillingAndOriginalPath(t *testing.T) {
	repo := newMigrationsRepo(t)
	db := repo.GetDB()
	user := &UserRecord{Username: "channel-repair", PasswordHash: "test-only", Role: "user", Status: "active", Balance: 20}
	key := &APIKeyRecord{Key: "channel-repair-test-key", TenantID: "channel-repair", Status: "active"}
	if err := repo.CreateUserWithAPIKey(user, key); err != nil {
		t.Fatal(err)
	}
	rec := &UsageLogRecord{TraceID: "repair-billed", Model: "repair-model", Channel: "backup", APIKey: key.Key, TenantID: key.TenantID, StatusCode: 200, Cost: 0.125000001, PromptTokens: 30, CompletionTokens: 11, TotalTokens: 41}
	if err := repo.RecordUsageLogContext(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	// Recreate v51's schema and FTS index, then its legacy channel value.
	for _, suffix := range []string{"ai", "ad", "au"} {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS usage_logs_fts_` + suffix); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{`DROP TABLE usage_logs_fts`, `ALTER TABLE usage_logs DROP COLUMN channel_trail`, `ALTER TABLE request_traces DROP COLUMN channel_trail`} {
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := addUsageLogFTSIndex(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE usage_logs SET channel='primary→backup' WHERE billing_id=?`, rec.BillingID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO request_traces(trace_id,channel,status_code) VALUES('repair-trace','primary→backup',200)`); err != nil {
		t.Fatal(err)
	}
	type moneyState struct{ logs, units, tokens, settlements, settled, balance, keyUsed int64 }
	snapshot := func() moneyState {
		var s moneyState
		if err := db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(cost_units),0),COALESCE(SUM(total_tokens),0) FROM usage_logs`).Scan(&s.logs, &s.units, &s.tokens); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(cost_units),0) FROM billing_settlements`).Scan(&s.settlements, &s.settled); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT balance_units FROM users WHERE id=?`, user.ID).Scan(&s.balance); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT used_cost_units FROM api_keys WHERE id=?`, key.ID).Scan(&s.keyUsed); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	if before.settlements != 1 || before.settled != 125000001 || before.balance != ToUnits(20)-125000001 {
		t.Fatalf("billing fixture not settled: %+v", before)
	}
	for range 2 {
		if err := separateChannelTrails(db); err != nil {
			t.Fatal(err)
		}
	}
	if got := snapshot(); got != before {
		t.Fatalf("cleanup changed billing: before=%+v after=%+v", before, got)
	}
	logs, err := repo.ListUsageLogsWithFilter(LogFilter{TraceID: "repair-billed"})
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs=%+v err=%v", logs, err)
	}
	if logs[0].Channel != "backup" || logs[0].ChannelTrail != "primary→backup" {
		t.Fatalf("identity=%+v", logs[0])
	}
	traces, err := repo.ListRequestTraces(TraceFilter{TraceID: "repair-trace"})
	if err != nil || len(traces) != 1 || traces[0].Channel != "backup" || traces[0].ChannelTrail != "primary→backup" {
		t.Fatalf("trace=%+v err=%v", traces, err)
	}
	if got := countLogsWithKeyword(t, repo, "primary"); got != 1 {
		t.Fatalf("migration lost path search: %d", got)
	}
}

func TestNewAuditPathsHaveOneSupplierAndRetainRetryDiagnostics(t *testing.T) {
	repo := newMigrationsRepo(t)
	now := time.Now()
	logs := []*UsageLogRecord{
		{TraceID: "failover", Model: "alias", Channel: "primary→backup", StatusCode: 200, CreatedAt: now},
		{TraceID: "retry", Model: "alias", Channel: "backup→backup", StatusCode: 200, CreatedAt: now},
		{TraceID: "other", Model: "alias", Channel: "backup-extra", StatusCode: 200, CreatedAt: now},
	}
	if err := repo.BatchRecordUsageLogsContext(context.Background(), logs); err != nil {
		t.Fatal(err)
	}
	facets, err := repo.ListUsageLogFacets(LogFilter{})
	if err != nil || !reflect.DeepEqual(facets.Channels, []string{"backup", "backup-extra"}) {
		t.Fatalf("phantom channel facets: %+v err=%v", facets, err)
	}
	report, err := repo.GetSupplierReconciliation(ChannelReconciliationFilter{Channel: "backup"})
	if err != nil || report.Summary.TotalRequests != 2 || len(report.Suppliers) != 1 || report.Suppliers[0].ChannelName != "backup" {
		t.Fatalf("supplier attribution=%+v err=%v", report, err)
	}
	primary, err := repo.GetSupplierReconciliation(ChannelReconciliationFilter{Channel: "primary"})
	if err != nil || primary.Summary.TotalRequests != 0 {
		t.Fatalf("attempted channel counted as terminal supplier: %+v err=%v", primary, err)
	}
	activity, err := repo.GetRoutingActivity("alias", now.Add(-time.Hour))
	if err != nil || activity.Requests != 3 || activity.Failovers != 1 || len(activity.Recent) != 2 {
		t.Fatalf("same-channel retry counted as failover: %+v err=%v", activity, err)
	}
	for _, event := range activity.Recent {
		if event.Channel != "backup" || event.ChannelTrail == "" {
			t.Fatalf("path missing from event: %+v", event)
		}
	}
	if err := repo.RecordRequestTrace(&RequestTraceRecord{TraceID: "new-trace", Channel: "primary→backup", StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	traces, err := repo.ListRequestTraces(TraceFilter{TraceID: "new-trace"})
	if err != nil || len(traces) != 1 || traces[0].Channel != "backup" || traces[0].ChannelTrail != "primary→backup" {
		t.Fatalf("trace=%+v err=%v", traces, err)
	}
	// Search indexes must follow updates to the path as well as canonical names.
	if _, err := repo.GetDB().Exec(`UPDATE usage_logs SET channel_trail='alternate→backup' WHERE trace_id='failover'`); err != nil {
		t.Fatal(err)
	}
	if countLogsWithKeyword(t, repo, "primary") != 0 || countLogsWithKeyword(t, repo, "alternate") != 1 {
		t.Fatal("stale FTS path after update")
	}
	if _, err := repo.GetDB().Exec(`DELETE FROM usage_logs WHERE trace_id='failover'`); err != nil {
		t.Fatal(err)
	}
	if countLogsWithKeyword(t, repo, "alternate") != 0 {
		t.Fatal("stale FTS path after deletion")
	}
}
