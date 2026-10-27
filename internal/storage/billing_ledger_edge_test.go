package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// A charge with no owner row and no tenant used to disappear: the ledger row was
// written (so the delivery was not retried) but no wallet moved and nothing was
// reported.
func TestUnattributedChargeIsReportedAndStaysConsistent(t *testing.T) {
	r, _, _ := billingIDTestRepo(t)
	ctx := context.Background()

	record := billingRecord(&APIKeyRecord{Key: "sk-orphan-edge"}, "", "tr-orphan-edge", "bill-orphan-edge", 2.00)
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{record}); err != nil {
		t.Fatalf("settle unattributed charge: %v", err)
	}

	// The ledger keeps the charge (so redelivery is a no-op) and the audit row
	// exists with its cost, so settlement-vs-audit stays in agreement.
	var settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = 'bill-orphan-edge'`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("settlement rows = %d, want 1", settlements)
	}
	var auditCost float64
	if err := r.db.QueryRow(`SELECT cost FROM usage_logs WHERE billing_id = 'bill-orphan-edge'`).Scan(&auditCost); err != nil {
		t.Fatal(err)
	}
	if auditCost != 2.00 {
		t.Fatalf("audit cost = %v, want 2.00", auditCost)
	}

	// The uncollected money is visible on /metrics instead of vanishing.
	if metrics := telemetry.GlobalMetrics.ToPrometheusFormat(); !strings.Contains(metrics, "airoute_billing_unattributed_total ") ||
		strings.Contains(metrics, "airoute_billing_unattributed_total 0") {
		t.Fatalf("unattributed counter was not published:\n%s", metrics)
	}

	// A redelivery must not charge or audit twice.
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(&APIKeyRecord{Key: "sk-orphan-edge"}, "", "tr-orphan-edge", "bill-orphan-edge", 2.00),
	}); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = 'bill-orphan-edge'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("audit rows after redelivery = %d, want 1", rows)
	}
}

// Pre-billing_id rows are recorded in the settlement ledger as "legacy-<id>"
// while usage_logs.billing_id stays NULL. Reconciliation has to use the same
// expression, otherwise those rows can never be linked or repaired.
func TestLegacySettlementKeysAreRepairedAndLinked(t *testing.T) {
	r, _, _ := billingIDTestRepo(t)

	// A legacy audit row: billing_id NULL, charged, old enough to pass the repair
	// grace window, and (like a rolling-deploy row) with no ledger entry yet.
	created := time.Now().Add(-time.Hour).UTC().Format(sqliteTimestampLayout)
	res, err := r.db.Exec(`INSERT INTO usage_logs (trace_id, api_key, tenant_id, model, prompt_tokens, total_tokens, cost, cost_units, status_code, created_at, billing_id)
		VALUES ('tr-legacy-link', 'sk-paid', 'paid-user', 'test-model', 1000000, 1000000, 3, ?, 200, ?, NULL)`,
		ToUnits(3), created)
	if err != nil {
		t.Fatal(err)
	}
	legacyID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	legacyKey := "legacy-" + strconv.FormatInt(legacyID, 10)

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()

	var ledger int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = ?`, legacyKey).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 {
		t.Fatalf("legacy audit row was not repaired into the ledger (key %s)", legacyKey)
	}
	// Linkage: the settlement row now matches the audit row through the legacy key,
	// so nothing is reported as unlinked.
	if got := telemetry.GlobalMetrics.BillingLedgerUnlinkedUnits.Load(); got != 0 {
		t.Fatalf("unlinked ledger units = %d, want 0 (legacy keys must link)", got)
	}
	if got := telemetry.GlobalMetrics.BillingAuditLagUnits.Load(); got != 0 {
		t.Fatalf("audit lag = %d, want 0 after the legacy repair", got)
	}
}

// Retention legitimately deletes charged audit rows. Before the pruned-units
// baseline the audit-lag gauge reported the entire pruned history forever.
func TestAuditLagSurvivesRetentionPruning(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 2; i++ {
		record := billingRecord(key, u.Username, "tr-prune-"+strconv.Itoa(i), "bill-prune-"+strconv.Itoa(i), 1.50)
		record.CreatedAt = old.Add(time.Duration(i) * time.Minute)
		if err := r.RecordUsageLogContext(ctx, record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()
	if got := telemetry.GlobalMetrics.BillingAuditLagUnits.Load(); got != 0 {
		t.Fatalf("audit lag before pruning = %d, want 0", got)
	}

	deleted, err := r.PurgeUsageLogsBefore(time.Now().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("pruned %d rows, want 2", deleted)
	}

	worker.runOnce()
	if got := telemetry.GlobalMetrics.BillingAuditLagUnits.Load(); got != 0 {
		t.Fatalf("audit lag after pruning = %d, want 0: pruning is expected, not a discrepancy", got)
	}
	if got := telemetry.GlobalMetrics.BillingAuditPrunedUnits.Load(); got != ToUnits(3.00) {
		t.Fatalf("pruned baseline = %d units, want %d", got, ToUnits(3.00))
	}
	if got := r.UsageAuditPrunedUnits(); got != ToUnits(3.00) {
		t.Fatalf("persisted pruned baseline = %d units, want %d", got, ToUnits(3.00))
	}

	// An unexplained loss (a ledger row disappearing without the baseline moving)
	// must still be reported.
	if _, err := r.db.Exec(`DELETE FROM billing_settlements WHERE billing_id = 'bill-prune-0'`); err != nil {
		t.Fatal(err)
	}
	worker.runOnce()
	if got := telemetry.GlobalMetrics.BillingAuditLagUnits.Load(); got != ToUnits(1.50) {
		t.Fatalf("audit lag after an unexplained ledger loss = %d, want %d", got, ToUnits(1.50))
	}
}

// Deleting a charged audit row does not reverse the money: it only destroys the
// evidence. The delete endpoints must refuse charged rows and say so.
func TestChargedUsageLogsCannotBeDeleted(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	charged := billingRecord(key, u.Username, "tr-charged", "bill-charged", 2.00)
	if err := r.RecordUsageLogContext(ctx, charged); err != nil {
		t.Fatal(err)
	}
	var chargedID int64
	if err := r.db.QueryRow(`SELECT id FROM usage_logs WHERE billing_id = 'bill-charged'`).Scan(&chargedID); err != nil {
		t.Fatal(err)
	}
	free := &UsageLogRecord{TraceID: "tr-free", BillingID: "bill-free-edge", StatusCode: 500}
	if err := r.RecordUsageLogContext(ctx, free); err != nil {
		t.Fatal(err)
	}
	var freeID int64
	if err := r.db.QueryRow(`SELECT id FROM usage_logs WHERE billing_id = 'bill-free-edge'`).Scan(&freeID); err != nil {
		t.Fatal(err)
	}

	if err := r.DeleteUsageLog(chargedID); !errors.Is(err, ErrChargedUsageLogImmutable) {
		t.Fatalf("deleting a charged row returned %v, want ErrChargedUsageLogImmutable", err)
	}
	var stillThere int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE id = ?`, chargedID).Scan(&stillThere); err != nil {
		t.Fatal(err)
	}
	if stillThere != 1 {
		t.Fatal("the charged audit row was deleted anyway")
	}
	if err := r.DeleteUsageLog(freeID); err != nil {
		t.Fatalf("deleting an uncharged row: %v", err)
	}

	// A second uncharged row for the batch case (the first one was already removed
	// by the single delete above).
	secondFree := &UsageLogRecord{TraceID: "tr-free-2", BillingID: "bill-free-edge-2", StatusCode: 500}
	if err := r.RecordUsageLogContext(ctx, secondFree); err != nil {
		t.Fatal(err)
	}
	var secondFreeID int64
	if err := r.db.QueryRow(`SELECT id FROM usage_logs WHERE billing_id = 'bill-free-edge-2'`).Scan(&secondFreeID); err != nil {
		t.Fatal(err)
	}

	deleted, skipped, err := r.BatchDeleteUsageLogs([]int64{chargedID, secondFreeID})
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if deleted != 1 || skipped != 1 {
		t.Fatalf("batch delete = %d deleted / %d skipped, want 1/1", deleted, skipped)
	}

	deleted, skipped, err = r.ClearAllUsageLogs()
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if deleted != 0 || skipped != 1 {
		t.Fatalf("clear = %d deleted / %d skipped, want 0/1 (the charged row must survive)", deleted, skipped)
	}
}

// The spill directory is shared by every replica. A starting replica used to
// replay and delete a peer's live file, losing every record the peer appended
// afterwards.
func TestSpillReplayDoesNotStealALiveReplicaFile(t *testing.T) {
	t.Setenv("GATEWAY_USAGE_SPILL_CLAIM_GRACE_SEC", "900")
	r := accountWalletRepo(t)
	dir := t.TempDir()

	writeSpill := func(name string, billingID string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		line := `{"trace_id":"tr-` + billingID + `","api_key":"sk-spill","tenant_id":"","model":"m","prompt_tokens":1,"total_tokens":1,"cost":0.1,"status_code":200,"billing_id":"` + billingID + `"}` + "\n"
		if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	livePeer := writeSpill("usage_spill-host-4242-111111.jsonl", "bill-live-peer")
	stalePeer := writeSpill("usage_spill-host-9999-222222.jsonl", "bill-stale-peer")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(stalePeer, old, old); err != nil {
		t.Fatal(err)
	}

	spill := &usageSpill{dir: dir, maxBytes: 8 << 20, path: filepath.Join(dir, "usage_spill-own-process.jsonl")}
	replayed, err := spill.replay(r)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed != 1 {
		t.Fatalf("replayed %d records, want 1 (only the quiet peer's file)", replayed)
	}
	if _, err := os.Stat(livePeer); err != nil {
		t.Fatalf("a live replica's spill file was consumed: %v", err)
	}
	if _, err := os.Stat(stalePeer); !os.IsNotExist(err) {
		t.Fatalf("the quiet peer's spill file was not consumed (stat err = %v)", err)
	}

	// Only the quiet peer's charge was applied.
	var charged int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&charged); err != nil {
		t.Fatal(err)
	}
	if charged != 1 {
		t.Fatalf("settlement rows = %d, want 1", charged)
	}

	// Once the live peer goes quiet, its file becomes claimable.
	if err := os.Chtimes(livePeer, old, old); err != nil {
		t.Fatal(err)
	}
	if replayed, err = spill.replay(r); err != nil || replayed != 1 {
		t.Fatalf("second replay = %d records (err %v), want 1", replayed, err)
	}
}
