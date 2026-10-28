package storage

import (
	"testing"
	"time"
)

// insertSettlementRow inserts a ledger row directly so the test controls both
// sides of the comparison (settleCharges would create one for every usage row).
func insertSettlementRow(t *testing.T, repo *Repository, billingID, tenant string, units, tokens int64, at time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		billingID, "digest-"+billingID, tenant, units, tokens, repo.dialectTimePrecise(at)); err != nil {
		t.Fatalf("insert settlement %s: %v", billingID, err)
	}
}

// insertAuditRow inserts an audit row directly (bypassing settlement) so the
// ledger side can disagree on purpose.
func insertAuditRow(t *testing.T, repo *Repository, billingID, tenant string, units, tokens int64, at time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(`INSERT INTO usage_logs
		(trace_id, api_key, tenant_id, model, cost_units, total_tokens, status_code, created_at, billing_id)
		VALUES (?, ?, ?, ?, ?, ?, 200, ?, ?)`,
		"tr-"+billingID, "digest-"+billingID, tenant, "m", units, tokens, repo.dialectTimePrecise(at), billingID); err != nil {
		t.Fatalf("insert audit row %s: %v", billingID, err)
	}
}

// E-03: tenant_id and total_tokens on the settlement ledger now have a real read
// path — a per-tenant comparison against the audit trail inside a pruning-safe
// window.
func TestTenantLedgerDriftReadsSettlementAttribution(t *testing.T) {
	repo := newDialectRepo(t)
	// Outside billingSettlementRepairGrace so in-flight rows are not simulated, and
	// well inside the retention window so pruning cannot look like drift.
	at := time.Now().Add(-time.Hour)

	// Tenant t1 agrees: 100 units / 1000 tokens on both sides.
	insertSettlementRow(t, repo, "bill-t1", "t1", 100, 1000, at)
	insertAuditRow(t, repo, "bill-t1", "t1", 100, 1000, at)

	// Tenant t2 disagrees by 20 units; its token total is only reported.
	insertSettlementRow(t, repo, "bill-t2", "t2", 50, 500, at)
	insertAuditRow(t, repo, "bill-t2", "t2", 30, 300, at)

	reconciler := &BillingReconciler{repo: repo}
	driftUnits, driftCount, tenants, err := reconciler.tenantLedgerDrift()
	if err != nil {
		t.Fatalf("tenant ledger drift: %v", err)
	}
	if driftUnits != 20 {
		t.Fatalf("driftUnits = %d, want 20", driftUnits)
	}
	if driftCount != 1 {
		t.Fatalf("driftCount = %d, want 1", driftCount)
	}
	if len(tenants) != 2 {
		t.Fatalf("tenant rows = %d, want 2", len(tenants))
	}
	// Ordered by settled money descending.
	if tenants[0].Tenant != "t1" || tenants[0].SettledUnits != 100 || tenants[0].AuditUnits != 100 {
		t.Fatalf("unexpected first tenant row: %+v", tenants[0])
	}
	if tenants[1].Tenant != "t2" || tenants[1].SettledUnits != 50 || tenants[1].AuditUnits != 30 {
		t.Fatalf("unexpected second tenant row: %+v", tenants[1])
	}
	// total_tokens is read, not just written.
	if tenants[0].SettledTokens != 1000 || tenants[1].SettledTokens != 500 {
		t.Fatalf("settled token totals were not read: %+v", tenants)
	}
}

// The comparison must ignore rows newer than the settlement grace window,
// otherwise every reconcile pass would flag charges that are still in flight.
func TestTenantLedgerDriftIgnoresInFlightRows(t *testing.T) {
	repo := newDialectRepo(t)
	fresh := time.Now() // inside the grace window
	insertSettlementRow(t, repo, "bill-inflight", "t1", 10, 100, fresh)

	reconciler := &BillingReconciler{repo: repo}
	driftUnits, _, _, err := reconciler.tenantLedgerDrift()
	if err != nil {
		t.Fatalf("tenant ledger drift: %v", err)
	}
	if driftUnits != 0 {
		t.Fatalf("in-flight settlement reported as %d units of drift", driftUnits)
	}
}
