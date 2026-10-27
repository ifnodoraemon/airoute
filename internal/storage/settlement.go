package storage

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// ErrSettlementNotInitialized is returned when the synchronous settlement
// runtime has not been installed (unit tests, CLI subcommands). Callers treat it
// as "no local settlement available" rather than as a billing failure: the
// asynchronous pipeline still applies the charge.
var ErrSettlementNotInitialized = errors.New("billing settlement is not initialized")

// settlementRepo is installed once at startup by InitBillingSettlement.
var settlementRepo *Repository

// InitBillingSettlement enables synchronous charge settlement for the data plane.
func InitBillingSettlement(repo *Repository) {
	settlementRepo = repo
}

// SettleChargeNow applies a charge's wallet movement immediately, without waiting
// for the audit log to be persisted.
//
// This is the second half of the billing/audit decoupling: the money side used to
// live inside the audit write transaction, so a healthy database but an unhealthy
// log pipeline meant a delayed charge. Now the data plane settles at request
// completion and the audit row follows asynchronously. If this call fails (for
// example the database is unreachable) the record still travels through
// Redis/spill and is settled later, so a failure here degrades to the previous
// behaviour instead of losing the charge.
func SettleChargeNow(ctx context.Context, rec *UsageLogRecord) error {
	repo := settlementRepo
	if repo == nil {
		return ErrSettlementNotInitialized
	}
	err := repo.SettleUsageCharge(ctx, rec)
	switch {
	case err == nil:
		telemetry.GlobalMetrics.BillingSettledSynchronously.Add(1)
	case errors.Is(err, ErrSettlementNotInitialized):
		// no settlement runtime installed; nothing to count
	default:
		countDeferredSettlement(err)
	}
	return err
}

// SettleUsageCharge applies one usage record's charge exactly once.
func (r *Repository) SettleUsageCharge(ctx context.Context, rec *UsageLogRecord) error {
	if rec == nil {
		return nil
	}
	normalizeUsageLog(rec)
	return r.settleCharges(ctx, []*UsageLogRecord{rec})
}

// settleCharges applies the money movement for a batch of usage records.
//
// A charge is applied only when its billing_id is newly inserted into
// billing_settlements, which makes this call idempotent across every path that
// can deliver a record (synchronous settlement, Redis redelivery, spill replay,
// local retry). Records already settled are marked so later calls skip them.
func (r *Repository) settleCharges(ctx context.Context, logs []*UsageLogRecord) error {
	pending := make([]*UsageLogRecord, 0, len(logs))
	for _, log := range logs {
		if log == nil || log.Settled {
			continue
		}
		if log.PricingSnapshot != nil && log.Cost > 0 {
			if err := ValidatePriceCurrency(&ModelPriceRecord{Currency: log.PricingSnapshot.Currency}); err != nil {
				// One charge that cannot be expressed in the wallet's currency must
				// not fail the whole delivery: the pipeline is at-least-once, so
				// returning an error here left every other record in the batch
				// unpersisted and uncharged while the consumer retried this one
				// message forever. Close the record out deterministically instead.
				rejectUnbillableCharge(log, err)
				continue
			}
		}
		if log.Cost <= 0 && log.TotalTokens <= 0 {
			// Nothing to move; mark it so the writer does not reconsider it.
			log.Settled = true
			continue
		}
		if log.BillingID == "" {
			// The transport normally stamps a stable key (stream entry id, spill
			// line, or AsyncLogger.Record). Deriving it from the record keeps a
			// redelivery of a keyless payload idempotent; minting a new random key
			// per delivery charged the same request twice.
			log.BillingID = DerivedBillingID(log)
		}
		pending = append(pending, log)
	}
	if len(pending) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	settledIDs, err := r.insertSettlementRows(tx, pending)
	if err != nil {
		return err
	}
	if len(settledIDs) == 0 {
		// Every record was settled by an earlier delivery.
		if err := tx.Commit(); err != nil {
			return err
		}
		for _, log := range pending {
			log.Settled = true
		}
		return nil
	}

	keyCost := make(map[string]float64)
	keyTokens := make(map[string]int64)
	keyTenant := make(map[string]string)
	orphanTenantCost := make(map[string]float64)

	for _, log := range pending {
		if _, fresh := settledIDs[log.BillingID]; !fresh {
			// Settled earlier; nothing to move for this record.
			log.Settled = true
			continue
		}
		if log.APIKey != "" {
			keyCost[log.APIKey] += log.Cost
			keyTokens[log.APIKey] += int64(log.TotalTokens)
			if log.TenantID != "" {
				keyTenant[log.APIKey] = log.TenantID
			}
		} else if log.TenantID != "" && log.Cost > 0 {
			orphanTenantCost[log.TenantID] += log.Cost
		}
	}

	if err := r.applyChargeAggregates(tx, keyCost, keyTokens, keyTenant, orphanTenantCost); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	for _, log := range pending {
		log.Settled = true
	}
	return nil
}

// settlementColumns is the column list used by the batched insert. Six columns
// per row keeps chunks far below SQLite's bound-parameter limit.
const (
	settlementColumns  = `billing_id, api_key, tenant_id, cost_units, total_tokens, created_at`
	settlementColumnsN = 6
	settlementChunk    = 100
)

// insertSettlementRows records the charges that are being applied now and
// returns the billing_ids that were newly inserted (and therefore still need
// their money moved).
func (r *Repository) insertSettlementRows(tx *Tx, logs []*UsageLogRecord) (map[string]struct{}, error) {
	args := make([]any, 0, len(logs)*settlementColumnsN)
	index := make([]string, 0, len(logs))
	seen := make(map[string]struct{}, len(logs))

	for _, log := range logs {
		if _, duplicate := seen[log.BillingID]; duplicate {
			continue
		}
		seen[log.BillingID] = struct{}{}
		createdAt := log.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		args = append(args,
			log.BillingID, log.APIKey, log.TenantID, ToUnits(log.Cost), int64(log.TotalTokens),
			createdAt.UTC().Format(sqliteTimestampLayout),
		)
		index = append(index, log.BillingID)
	}

	settled := make(map[string]struct{}, len(index))
	rowPlaceholder := "(" + strings.Repeat("?, ", settlementColumnsN-1) + "?)"
	for start := 0; start < len(index); start += settlementChunk {
		end := start + settlementChunk
		if end > len(index) {
			end = len(index)
		}
		placeholders := make([]string, end-start)
		for i := range placeholders {
			placeholders[i] = rowPlaceholder
		}
		query := `INSERT INTO billing_settlements (` + settlementColumns + `) VALUES ` + strings.Join(placeholders, ",") + ` ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id`
		rows, err := tx.Query(query, args[start*settlementColumnsN:end*settlementColumnsN]...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var billingID string
			if err := rows.Scan(&billingID); err != nil {
				rows.Close()
				return nil, err
			}
			settled[billingID] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return settled, nil
}

// applyChargeAggregates updates key quota counters and deducts the charged amount
// from the owning wallet (with the tenant fallback and admin exemption rules).
func (r *Repository) applyChargeAggregates(tx *Tx, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64) error {
	if len(keyCost) > 0 {
		updateKeySQL := r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_units = COALESCE(used_cost_units, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`)
		updateKeyStmt, err := tx.Prepare(updateKeySQL)
		if err != nil {
			return err
		}
		defer updateKeyStmt.Close()
		for key, cost := range keyCost {
			tokens := keyTokens[key]
			if cost > 0 || tokens > 0 {
				if _, err := updateKeyStmt.Exec(cost, ToUnits(cost), tokens, key); err != nil {
					return err
				}
			}
		}
	}

	for key, cost := range keyCost {
		if cost <= 0 {
			continue
		}
		applied, _, err := r.deductWallet(tx,
			`id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`, []any{key}, cost)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		var isOwnerAdmin bool
		row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), key)
		if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
			// Admin keys are exempt; do not fallback to tenant.
			continue
		}
		if tenant := keyTenant[key]; tenant != "" {
			orphanTenantCost[tenant] += cost
		}
	}

	for tenant, cost := range orphanTenantCost {
		if cost <= 0 {
			continue
		}
		if _, _, err := r.deductWallet(tx,
			`(username = ? OR email = ?) AND role != 'admin'`, []any{tenant, tenant}, cost); err != nil {
			return err
		}
	}
	return nil
}

// rejectUnbillableCharge closes out a record whose charge cannot be applied in
// the wallet's currency (a legacy foreign-currency price row).
//
// It is deliberately not an error. The record keeps its real token counts, its
// pricing snapshot and the rejection reason, so the audit trail still explains
// what happened; only the money columns are zeroed, which keeps the settlement
// ledger, the key counters and the audit trail in agreement. The alternative —
// charging a USD number as if it were CNY — overcharges the customer by the
// exchange rate, and returning an error (the previous behaviour) discarded the
// whole batch.
func rejectUnbillableCharge(log *UsageLogRecord, reason error) {
	rejectedCost := log.Cost
	currency := ""
	if log.PricingSnapshot != nil {
		currency = log.PricingSnapshot.Currency
	}
	log.Cost = 0
	log.Settled = true
	if log.ErrorMessage == "" {
		log.ErrorMessage = "billing rejected: " + reason.Error()
	}

	telemetry.GlobalMetrics.BillingChargesRejected.Add(1)
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-charge-rejected", 30*time.Second,
		"charge rejected: the pricing snapshot is not settled in CNY; the request is audited but not billed",
		"reason", reason.Error(),
		"currency", currency,
		"model", log.Model,
		"trace_id", log.TraceID,
		"rejected_cost_cny", rejectedCost,
		"total_tokens", log.TotalTokens)
}

// countDeferredSettlement is called when synchronous settlement could not run;
// the charge is still delivered through the async pipeline.
func countDeferredSettlement(err error) {
	telemetry.GlobalMetrics.BillingSettlementsDeferred.Add(1)
	if errors.Is(err, ErrSettlementNotInitialized) {
		return
	}
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "settle-deferred", 30*time.Second,
		"synchronous settlement failed; the charge is deferred to the async pipeline",
		"error", err.Error())
}
