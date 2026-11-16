package storage

import (
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// ledgerLinkage reports settled units that no audit row can be matched to.
//
// The join uses usageLogBillingIDExpr, so the pre-billing_id rows recorded as
// "legacy-<id>" link correctly instead of looking orphaned. Only rows inside the
// retention window are counted: older rows are expected to have been pruned by
// now, and flagging them would make the gauge unusable. A non-zero value means a
// charge reached the ledger while its audit row never arrived (or was deleted
// outside the guarded endpoints).
//
// Query shape: the created_at bound is not only the retention filter, it is what
// keeps this query proportional to the retention window instead of the whole
// ledger (idx_billing_settlements_created). Ledger rows retired by the retention
// worker always fall outside that window — a row only becomes retirable once its
// audit evidence is gone — so the archive aggregate does not have to be counted
// here.
func (b *BillingReconciler) ledgerLinkage() (units int64, rows int64, err error) {
	retention, _ := ResolveUsageLogRetention()
	cutoff := time.Time{}
	if retention > 0 {
		cutoff = time.Now().Add(-retention)
	}
	cutoffValue := b.repo.dialectTimePrecise(cutoff)
	query := b.repo.db.Rebind(`SELECT COUNT(*), COALESCE(SUM(s.cost_units), 0)
		FROM billing_settlements s
		WHERE s.created_at >= ?
		  AND NOT EXISTS (
			SELECT 1 FROM usage_logs l WHERE ` + usageLogBillingIDExpr + ` = s.billing_id
		  )`)
	if err := b.repo.db.QueryRow(query, cutoffValue).Scan(&rows, &units); err != nil {
		return 0, 0, fmt.Errorf("ledger linkage query: %w", err)
	}
	return units, rows, nil
}

// publishWalletTotal reports the integer funds against which reservations run.
func (b *BillingReconciler) publishWalletTotal() error {
	var total int64
	if err := b.repo.db.QueryRow(`SELECT COALESCE(SUM(balance_units), 0) FROM users`).Scan(&total); err != nil {
		return fmt.Errorf("wallet total query: %w", err)
	}
	telemetry.GlobalMetrics.BillingWalletTotalUnits.Store(total)
	return nil
}

// tenantLedgerWindow bounds the per-tenant settlement-vs-audit comparison.
//
// It has to be shorter than the audit retention window (365 days by default) so
// that pruning an expired usage row can never masquerade as tenant drift, and it
// ends before billingSettlementRepairGrace so a row still travelling through the
// async pipeline is not counted as missing.
const (
	tenantLedgerWindow = 7 * 24 * time.Hour
	tenantLedgerTopN   = 5
)

// tenantLedgerTotals is one tenant's settled and audited totals inside the
// reconciliation window.
type tenantLedgerTotals struct {
	Tenant        string
	SettledUnits  int64
	SettledTokens int64
	AuditUnits    int64
}

// tenantLedgerDrift reports, per tenant, the difference between the money the
// settlement ledger recorded and the money the audit trail recorded inside the
// reconciliation window.
//
// This is the read path for billing_settlements.tenant_id and
// billing_settlements.total_tokens: both are written by settlement for
// attribution, but the wallet-ledger view that consumed them was cut in v1.1, so
// without this comparison nothing ever read them. Only the cost side is treated
// as an invariant:
//
//   - a charge rejected for currency reasons zeroes its cost but keeps its token
//     counts and deliberately writes no ledger row, so token totals legitimately
//     differ for such rows;
//   - the token figures are therefore reported (summary log) rather than compared.
func (b *BillingReconciler) tenantLedgerDrift() (driftUnits int64, driftCount int, tenants []tenantLedgerTotals, err error) {
	until := time.Now().Add(-billingSettlementRepairGrace)
	since := until.Add(-tenantLedgerWindow)
	sinceArg := b.repo.dialectTimePrecise(since)
	untilArg := b.repo.dialectTimePrecise(until)

	// A UNION ALL of the two sources, summed per tenant, is portable to both
	// engines (SQLite has no FULL OUTER JOIN) and lets a tenant that only exists
	// on one side still show up as drift.
	query := b.repo.db.Rebind(`SELECT tenant_id,
			COALESCE(SUM(settled_units), 0) AS settled_units,
			COALESCE(SUM(settled_tokens), 0) AS settled_tokens,
			COALESCE(SUM(audit_units), 0) AS audit_units
		FROM (
			SELECT COALESCE(tenant_id, '') AS tenant_id, cost_units AS settled_units,
			       total_tokens AS settled_tokens, 0 AS audit_units
			  FROM billing_settlements
			 WHERE created_at >= ? AND created_at < ?
			UNION ALL
			SELECT COALESCE(tenant_id, ''), 0, 0, cost_units
			  FROM usage_logs
			 WHERE created_at >= ? AND created_at < ?
		) per_tenant
		GROUP BY tenant_id
		ORDER BY settled_units DESC, tenant_id ASC`)
	rowsRead, err := b.repo.db.Query(query, sinceArg, untilArg, sinceArg, untilArg)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("tenant ledger drift query: %w", err)
	}
	defer rowsRead.Close()

	tenants = make([]tenantLedgerTotals, 0, 16)
	for rowsRead.Next() {
		var row tenantLedgerTotals
		if err := rowsRead.Scan(&row.Tenant, &row.SettledUnits, &row.SettledTokens, &row.AuditUnits); err != nil {
			return 0, 0, nil, fmt.Errorf("scan tenant ledger drift: %w", err)
		}
		if diff := row.SettledUnits - row.AuditUnits; diff != 0 {
			if diff < 0 {
				diff = -diff
			}
			driftUnits += diff
			driftCount++
		}
		tenants = append(tenants, row)
	}
	if err := rowsRead.Err(); err != nil {
		return 0, 0, nil, fmt.Errorf("iterate tenant ledger drift: %w", err)
	}
	return driftUnits, driftCount, tenants, nil
}
