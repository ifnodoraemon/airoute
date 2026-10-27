package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// A charge whose pricing snapshot is not settled in CNY used to fail the whole
// delivery: settleCharges returned the validation error before the transaction,
// so every healthy record in the same batch was neither charged nor audited and
// the consumer retried the poison message forever.
func TestForeignCurrencyRecordDoesNotPoisonBatch(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	healthy := billingRecord(key, u.Username, "tr-healthy", "bill-healthy", 1.00)
	legacyUSD := billingRecord(key, u.Username, "tr-legacy-usd", "bill-legacy-usd", 5.00)
	legacyUSD.PricingSnapshot = &UsagePricingSnapshot{Model: "legacy-usd-model", Currency: "USD", PromptPrice: 3, CompletionPrice: 15}

	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{healthy, legacyUSD}); err != nil {
		t.Fatalf("a foreign-currency record must not fail its batch: %v", err)
	}

	// The healthy record is settled exactly as before.
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(9.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (the healthy record must still be charged)", user.BalanceUnits, want)
	}

	// Both records are audited: the rejected one is not silently dropped.
	var audits, settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("audit rows = %d, want 2 (the rejected charge must stay auditable)", audits)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("settlement rows = %d, want 1 (only the healthy charge moved money)", settlements)
	}

	// The rejected row keeps its token counts and snapshot as evidence, but cost 0
	// keeps the ledger, the key counters and the audit trail in agreement.
	var cost, promptTokens int64
	var costFloat float64
	var snapshot, message string
	if err := r.db.QueryRow(`SELECT prompt_tokens, cost, cost_units, COALESCE(pricing_snapshot, ''), COALESCE(error_message, '')
		FROM usage_logs WHERE billing_id = 'bill-legacy-usd'`).
		Scan(&promptTokens, &costFloat, &cost, &snapshot, &message); err != nil {
		t.Fatal(err)
	}
	_ = cost
	if costFloat != 0 {
		t.Fatalf("rejected charge persisted cost = %v, want 0", costFloat)
	}
	if promptTokens != 1_000_000 {
		t.Fatalf("rejected charge prompt tokens = %d, want the real count", promptTokens)
	}
	if !strings.Contains(snapshot, `"currency":"USD"`) {
		t.Fatalf("pricing snapshot = %q, want the rejected rate kept as evidence", snapshot)
	}
	if !strings.Contains(message, "billing rejected") {
		t.Fatalf("error_message = %q, want the rejection reason", message)
	}
	// The rejection is published: a price row that cannot settle in CNY has to be
	// visible to an operator, not just to a log line.
	if metrics := telemetry.GlobalMetrics.ToPrometheusFormat(); !strings.Contains(metrics, "airoute_billing_charges_rejected_total 1") {
		t.Fatalf("rejected-charge counter was not published:\n%s", metrics)
	}

	// A redelivery of the same poison record must not fail, duplicate the audit row
	// or charge twice.
	redelivered := billingRecord(key, u.Username, "tr-legacy-usd", "bill-legacy-usd", 5.00)
	redelivered.PricingSnapshot = &UsagePricingSnapshot{Model: "legacy-usd-model", Currency: "USD"}
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{redelivered}); err != nil {
		t.Fatalf("redelivered poison record must not error: %v", err)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("audit rows after redelivery = %d, want 2", audits)
	}
	user, _ = r.GetUserByID(u.ID)
	if want := ToUnits(9.00); user.BalanceUnits != want {
		t.Fatalf("balance units after redelivery = %d, want %d", user.BalanceUnits, want)
	}
}

// A payload that arrives without billing_id (written by a binary that predates
// the idempotency key) used to get a fresh random key per delivery, so a
// redelivery was a second wallet deduction.
func TestKeylessRecordRedeliveryChargesOnce(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	payload := func() *UsageLogRecord {
		rec := billingRecord(key, u.Username, "tr-keyless", "", 2.00)
		rec.CreatedAt = time.Unix(1_793_000_000, 123_456_789).UTC()
		return rec
	}

	for attempt := 1; attempt <= 3; attempt++ {
		if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{payload()}); err != nil {
			t.Fatalf("delivery %d: %v", attempt, err)
		}
	}

	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(8.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (three deliveries of one payload must charge once)", user.BalanceUnits, want)
	}
	var audits, settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit rows = %d, want 1", audits)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("settlement rows = %d, want 1", settlements)
	}
}

// A spill file written before billing_id existed is re-read from the top when a
// later batch fails (the file is only removed after a complete replay). Without a
// stable key the already-persisted prefix was charged again on every attempt.
func TestSpillReplayWithoutBillingIDChargesOnce(t *testing.T) {
	r, key, u := billingIDTestRepo(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "usage_spill-legacy-host-1-1.jsonl")
	// Exactly the shape an older binary marshalled: no billing_id, no settled flag.
	line := `{"id":0,"trace_id":"tr-spill","api_key":"` + key.Key + `","tenant_id":"` + u.Username +
		`","model":"test-model","prompt_tokens":1000000,"completion_tokens":0,"cached_tokens":0,` +
		`"total_tokens":1000000,"cost":3,"status_code":200}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	spill := &usageSpill{dir: dir, maxBytes: 8 << 20, path: filepath.Join(dir, "usage_spill-own.jsonl")}
	for attempt := 1; attempt <= 2; attempt++ {
		replayed, err := spill.replayFile(r, path)
		if err != nil {
			t.Fatalf("replay %d: %v", attempt, err)
		}
		if replayed != 1 {
			t.Fatalf("replay %d replayed %d records, want 1", attempt, replayed)
		}
	}

	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(7.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (replaying one spill line must charge once)", user.BalanceUnits, want)
	}
	var audits, settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit rows = %d, want 1", audits)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("settlement rows = %d, want 1", settlements)
	}
}

// A keyless record with no timestamp has no stable identity to derive from. It
// must still be persisted: collapsing two look-alike rows into one would silently
// lose an audit row, which is worse than losing idempotency for a caller that
// never reports when the charge happened.
func TestKeylessRecordWithoutTimestampIsNeverDropped(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	payload := func() *UsageLogRecord {
		// CreatedAt intentionally zero, as the direct repository callers in the
		// wild leave it.
		return billingRecord(key, u.Username, "", "", 1.00)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{payload()}); err != nil {
			t.Fatalf("delivery %d: %v", attempt, err)
		}
	}

	var audits int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("audit rows = %d, want 2: a record without a stable identity must not be deduplicated", audits)
	}
}
