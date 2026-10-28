package storage

import (
	"context"
	"errors"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"

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
//
// Transient PostgreSQL lock conflicts (deadlock detected, lock_not_available from
// our 500ms lock_timeout, statement cancellation) are retried up to
// settlementMaxRetries times with a short jittered backoff: the work is short and
// idempotent, and a batch that loses the row-lock race would otherwise defer every
// charge in it. Anything else is returned immediately — retrying a real error only
// makes the failure slower.
func (r *Repository) settleCharges(ctx context.Context, logs []*UsageLogRecord) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = r.settleChargesOnce(ctx, logs)
		if err == nil {
			return nil
		}
		if !isRetryableSettlementErr(err) {
			return err
		}
		telemetry.GlobalMetrics.SettlementLockTimeouts.Add(1)
		if attempt >= settlementMaxRetries || ctx.Err() != nil {
			return err
		}
		if !sleepSettlementRetry(ctx, settlementRetryDelay(attempt)) {
			return err
		}
	}
}

// settlementMaxRetries is the number of extra attempts after the first one.
// Two retries at 20–80ms cover the common "another flush holds the wallet row"
// case without holding the caller for long: the request-side wait is much longer.
const settlementMaxRetries = 2

// settlementRetryBaseDelay / settlementRetryMaxDelay bound the jittered backoff.
// The jitter matters as much as the delay: two flushes that collided on a row must
// not wake up together and collide again.
const (
	settlementRetryBaseDelay = 20 * time.Millisecond
	settlementRetryMaxDelay  = 80 * time.Millisecond
)

// isRetryableSettlementErr reports whether a settlement failure is a transient
// PostgreSQL lock/statement conflict:
//
//	40P01 deadlock_detected        — a concurrent flush formed a lock cycle
//	55P03 lock_not_available       — our lock_timeout=500 expired on a hot row
//	57014 query_canceled           — statement_timeout/cancellation aborted the work
//
// SQLite has no per-statement lock deadline (busy_timeout handles contention), so
// its errors are never retried here. The check is a pure function over the error
// so it can be unit tested without a PostgreSQL server.
func isRetryableSettlementErr(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	switch string(pqErr.Code) {
	case "40P01", "55P03", "57014":
		return true
	}
	return false
}

// settlementRetryDelay returns the jittered backoff before retry number attempt
// (0-based): uniformly distributed in [base, max].
func settlementRetryDelay(attempt int) time.Duration {
	span := settlementRetryMaxDelay - settlementRetryBaseDelay
	if span <= 0 {
		return settlementRetryBaseDelay
	}
	return settlementRetryBaseDelay + time.Duration(rand.Int64N(int64(span)+1))
}

// sleepSettlementRetry waits for d unless the caller goes away first; it reports
// whether the wait completed.
func sleepSettlementRetry(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// settleChargesOnce is one settlement attempt: everything is committed together or
// the transaction rolls back, so the retry wrapper always starts from a clean slate.
func (r *Repository) settleChargesOnce(ctx context.Context, logs []*UsageLogRecord) error {
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

	// Time the database transaction itself (begin → commit/rollback), not the
	// request-side wait: the batcher's queue wait is measured by its own counters,
	// and mixing the two would hide which half is slow.
	txStart := time.Now()
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
		telemetry.SettlementDurationSeconds.Observe(time.Since(txStart).Seconds())
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

	effects := &settlementEffects{}
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

	if err := r.applyChargeAggregates(tx, keyCost, keyTokens, keyTenant, orphanTenantCost, effects); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Only now is the movement durable; a rolled-back attempt must not count.
	effects.flush()

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
			r.dialectTimePrecise(createdAt),
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

// sortedMapKeys returns the map's keys in ascending order.
//
// Iterating a Go map is deliberately randomised, so two batches touching the same
// rows could lock A→B in one transaction and B→A in the next: that is a classic
// deadlock cycle (PostgreSQL 40P01, and a lock-wait pile-up on SQLite's single
// writer). Sorting the keys gives every transaction the same lock order.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// fallbackChargeToTenant routes an unapplied key charge the way the settlement
// rules require: an admin-owned key is exempt, a named tenant absorbs the charge
// on its own wallet, and anything else is reported as uncollected.
func fallbackChargeToTenant(key, tenant string, cost float64, ownerIsAdmin bool, orphanTenantCost map[string]float64) {
	if ownerIsAdmin {
		// Admin keys are exempt; do not fallback to tenant.
		return
	}
	if tenant != "" {
		orphanTenantCost[tenant] += cost
		return
	}
	// No owner row and no tenant to fall back to: the ledger row that was already
	// written records the charge as settled, but no wallet moved. Keep the two in
	// agreement (the delivery must not be retried) and make the uncollected money
	// visible instead of dropping it silently.
	reportUnattributedCharge(key, "", cost)
}

// applyChargeAggregates updates key quota counters and deducts the charged amount
// from the owning wallet (with the tenant fallback and admin exemption rules).
//
// The map keys are always traversed in sorted order so concurrent batches take the
// same locks in the same order.
func (r *Repository) applyChargeAggregates(tx *Tx, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64, effects *settlementEffects) error {
	keys := sortedMapKeys(keyCost)
	if len(keys) > 0 {
		var err error
		if r.db.Dialect() == "postgres" {
			err = r.applyPostgresChargeMerges(tx, keys, keyCost, keyTokens, keyTenant, orphanTenantCost, effects)
		} else {
			err = r.applyPortableChargeUpdates(tx, keys, keyCost, keyTokens, keyTenant, orphanTenantCost, effects)
		}
		if err != nil {
			return err
		}
	}

	for _, tenant := range sortedMapKeys(orphanTenantCost) {
		cost := orphanTenantCost[tenant]
		if cost <= 0 {
			continue
		}
		applied, _, err := r.deductWallet(tx,
			`(username = ? OR email = ?) AND role != 'admin'`, []any{tenant, tenant}, cost, effects)
		if err != nil {
			return err
		}
		if !applied {
			// The tenant named on the record no longer matches any billable user.
			reportUnattributedCharge("", tenant, cost)
		}
	}
	return nil
}

// applyPortableChargeUpdates is the SQLite path: one UPDATE api_keys per key, then
// one clamped UPDATE users per key.
//
// SQLite keeps the two statements: it has a single writer, so the extra round trip
// does not change concurrency, and its UPDATE ... FROM support is recent enough
// that folding the counter into the wallet statement would add a version
// dependency for no gain.
func (r *Repository) applyPortableChargeUpdates(tx *Tx, keys []string, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64, effects *settlementEffects) error {
	updateKeyStmt, err := tx.Prepare(r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_units = COALESCE(used_cost_units, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`))
	if err != nil {
		return err
	}
	defer updateKeyStmt.Close()
	for _, key := range keys {
		cost, tokens := keyCost[key], keyTokens[key]
		if cost <= 0 && tokens <= 0 {
			continue
		}
		if _, err := updateKeyStmt.Exec(cost, ToUnits(cost), tokens, key); err != nil {
			return err
		}
	}

	for _, key := range keys {
		cost := keyCost[key]
		if cost <= 0 {
			continue
		}
		applied, _, err := r.deductWallet(tx,
			`id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`, []any{key}, cost, effects)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		var ownerIsAdmin bool
		row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), key)
		if err := row.Scan(&ownerIsAdmin); err == nil && ownerIsAdmin {
			// Admin keys are exempt; do not fallback to tenant.
			continue
		}
		fallbackChargeToTenant(key, keyTenant[key], cost, false, orphanTenantCost)
	}
	return nil
}

// postgresChargeMergeSQL folds the three statements the settlement path used to
// issue per key — UPDATE api_keys counters, UPDATE users wallet, and (on refusal)
// SELECT users admin flag — into a single round trip.
//
// Equivalence with the portable path, piece by piece:
//   - key_update is exactly the old counter UPDATE (same columns, same expression).
//   - wallet_update is exactly deductWallet's users UPDATE: the same portable CASE
//     clamp, the same float mirror derived from the integer column, and the same
//     `role <> 'admin'` exemption. `AND ? > 0` reproduces deductWallet's early
//     return for a charge that rounds to zero units (it never touched a wallet and
//     fell through to the tenant fallback).
//   - owner_is_admin reproduces the old "SELECT (role = 'admin')" probe that only
//     ran after a refusal; NULL/absent users scan as 0, exactly like the discarded
//     scan error did before.
//   - the count columns distinguish "key row missing" from "wallet refused" without
//     another query, and balance_units feeds the waived/overdraft accounting.
//
// The lock order inside the statement is api_keys row → users row, the same order
// the two-statement path took, so the deadlock surface is unchanged (and now sorted
// across keys by the caller).
//
// Every placeholder is cast explicitly. lib/pq sends parameters as untyped text, so
// the server infers their types from context, and a bare `? > 0` infers *integer* —
// which overflows on a 2.5-CNY charge (2.5e9 units) with a confusing 22003 error
// instead of a settlement. The casts pin the intended types.
const postgresChargeMergeSQL = `
WITH key_update AS (
	UPDATE api_keys
	   SET used_cost = used_cost + CAST(? AS DOUBLE PRECISION),
	       used_cost_units = COALESCE(used_cost_units, 0) + CAST(? AS BIGINT),
	       used_tokens = used_tokens + CAST(? AS BIGINT),
	       updated_at = CURRENT_TIMESTAMP
	 WHERE key = CAST(? AS TEXT)
	 RETURNING user_id
), wallet_update AS (
	UPDATE users u
	   SET balance_units = CASE WHEN COALESCE(u.balance_units, 0) - CAST(? AS BIGINT) < CAST(? AS BIGINT) THEN CAST(? AS BIGINT) ELSE COALESCE(u.balance_units, 0) - CAST(? AS BIGINT) END,
	       balance = (CASE WHEN COALESCE(u.balance_units, 0) - CAST(? AS BIGINT) < CAST(? AS BIGINT) THEN CAST(? AS BIGINT) ELSE COALESCE(u.balance_units, 0) - CAST(? AS BIGINT) END) * 1.0 / 1000000000.0,
	       updated_at = CURRENT_TIMESTAMP
	  FROM key_update k
	 WHERE u.id = k.user_id
	   AND u.role <> 'admin'
	   AND CAST(? AS BIGINT) > 0
	 RETURNING u.balance_units
)
SELECT (SELECT COUNT(*) FROM key_update) AS key_rows,
       (SELECT COUNT(*) FROM wallet_update) AS wallet_rows,
       COALESCE((SELECT balance_units FROM wallet_update LIMIT 1), 0) AS balance_units,
       COALESCE((SELECT CASE WHEN u.role = 'admin' THEN 1 ELSE 0 END
                   FROM users u
                  WHERE u.id = (SELECT user_id FROM key_update)), 0) AS owner_is_admin`

// applyPostgresChargeMerges runs the merged statement once per key, in the sorted
// order the caller provides.
func (r *Repository) applyPostgresChargeMerges(tx *Tx, keys []string, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64, effects *settlementEffects) error {
	stmt, err := tx.Prepare(r.db.Rebind(postgresChargeMergeSQL))
	if err != nil {
		return err
	}
	defer stmt.Close()

	floor := -maxOverdraftUnits()
	for _, key := range keys {
		cost, tokens := keyCost[key], keyTokens[key]
		if cost <= 0 && tokens <= 0 {
			continue
		}
		units := ToUnits(cost)
		args := []any{
			cost, units, tokens, key,
			// balance_units assignment: units, floor, floor, units
			units, floor, floor, units,
			// balance float mirror: the same four placeholders
			units, floor, floor, units,
			// wallet_update guard: only deduct when the charge rounds to > 0 units
			units,
		}
		var keyRows, walletRows, balanceUnits, ownerIsAdmin int64
		if err := stmt.QueryRow(args...).Scan(&keyRows, &walletRows, &balanceUnits, &ownerIsAdmin); err != nil {
			return err
		}
		if walletRows > 0 {
			recordWalletDeductionEffect(cost, units, floor, balanceUnits, effects)
			continue
		}
		if cost <= 0 {
			// Counter-only key: the wallet was never asked to move.
			continue
		}
		fallbackChargeToTenant(key, keyTenant[key], cost, ownerIsAdmin == 1, orphanTenantCost)
	}
	return nil
}

// reportUnattributedCharge publishes a settled charge that no wallet could absorb.
//
// The settlement ledger deliberately still records it: the delivery is
// at-least-once, so leaving the row out would re-deliver and re-audit the charge
// forever, and the audit row exists with its cost. What was missing was any
// signal: without this counter and log an operator could not tell that money was
// being served but never collected.
func reportUnattributedCharge(apiKey, tenant string, cost float64) {
	telemetry.GlobalMetrics.BillingUnattributed.Add(1)
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-charge-unattributed", 30*time.Second,
		"charge settled with no billable wallet: the amount is uncollected",
		"api_key", apiKey,
		"tenant", tenant,
		"cost_cny", cost)
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
