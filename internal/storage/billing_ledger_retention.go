package storage

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// The settlement ledger used to only grow: audit rows have a retention policy
// (usage_retention.go) but billing_settlements has none, so it accumulated every
// charge forever, together with its indexes. This file adds an opt-in retention
// policy for it.
//
// The ledger is a financial document, so the policy is deliberately conservative:
//
//   - It is off by default (GATEWAY_BILLING_LEDGER_RETENTION_DAYS=0 keeps every
//     row). Deleting accounting evidence is never something a deploy should do on
//     its own.
//   - A row is only retired when its audit row is already gone AND the row is
//     older than the cutoff. The audit trail is what makes a charge explainable;
//     a ledger row whose audit row still exists is never touched, no matter how
//     old it is. (In practice that means the window shrinks to
//     min(ledger retention, audit retention) rows, because audit retention is
//     what removes the evidence in the first place.)
//   - Retiring a row must not change the reconciliation numbers. The units are
//     moved into billing_ledger_archives (one aggregate row per api_key, created
//     by migration 41) in the same transaction as the DELETE, and auditLag /
//     ledgerDrift add that aggregate back. That is why the code does not adjust
//     the "billing_audit_lag_baseline_units" value directly: that key stores the
//     lag itself, so adding the retired amount to it would make the next
//     incremental pass over-report the whole archived history. Keeping the money
//     in the archive table keeps the incremental and the full recalibration in
//     agreement (asserted by billing_reconcile_incremental_test.go).
//   - Every batch records the amount in system_settings
//     (billing_ledger_archive_<yyyymm>) and the pass logs a WARN, so an auditor
//     can explain the disappearance from the database alone.
//
// Batch size is 5000 rows per transaction: a ledger can be tens of millions of
// rows, and one giant DELETE would hold locks (and, on SQLite, the single writer)
// for the whole run.
//
// Why not DDL partitioning, which the audit suggested for this table? Switching
// billing_settlements to a partitioned table is a table rewrite that needs a
// maintenance window, and it changes the primary key: `id` alone stops being
// unique across partitions and the unique(billing_id) index that the settlement
// hot path uses as its exactly-once gate would have to include the partition key.
// That is a scheduled-window operation, not something a startup migration or a
// background worker may do. This file retires rows online instead; a partitioned
// layout can be adopted later with a shadow table + backfill, and this policy
// would still apply to the new table.
const (
	// Opt-in: 0 keeps every row.
	defaultBillingLedgerRetentionDays = 0
	defaultBillingLedgerPruneHours    = 24
	billingLedgerPruneBatchSize       = 5000
	// One pass never does more than this many batches, so a huge backlog cannot
	// hold the maintenance slot (or the WAL) for an unbounded time. The remainder
	// is picked up by the next pass.
	billingLedgerPruneMaxBatches = 200
	// Migrations, the reconcile pass (3m) and the audit prune worker (5m) start
	// first; this one is the least urgent.
	billingLedgerPruneStartupDelay = 15 * time.Minute
	// Monthly audit trail of retired amounts, e.g. billing_ledger_archive_202610.
	billingLedgerArchiveKeyPrefix = "billing_ledger_archive_"
	// migration 23 gave audit rows that predate billing_id the deterministic key
	// "legacy-<usage_logs.id>"; a ledger row with that shape links back to the
	// usage row's primary key.
	legacyBillingIDPrefix = "legacy-"
)

// ResolveBillingLedgerRetention reads the ledger retention window and prune
// cadence from the environment.
// GATEWAY_BILLING_LEDGER_RETENTION_DAYS=0 (the default) keeps every ledger row.
func ResolveBillingLedgerRetention() (retention time.Duration, interval time.Duration) {
	days := defaultBillingLedgerRetentionDays
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			days = value
		}
	}
	hours := defaultBillingLedgerPruneHours
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_BILLING_LEDGER_PRUNE_INTERVAL_HOURS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			hours = value
		}
	}
	return time.Duration(days) * 24 * time.Hour, time.Duration(hours) * time.Hour
}

// BillingLedgerRetention retires ledger rows whose audit evidence is already
// gone and that fall outside the configured window.
type BillingLedgerRetention struct {
	repo      *Repository
	retention time.Duration
	interval  time.Duration
	stop      chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

// InitBillingLedgerRetention starts the ledger retention worker. A zero window
// (the default) or a nil repository disables it, so no deployment deletes
// financial evidence unless it opts in.
func InitBillingLedgerRetention(repo *Repository) *BillingLedgerRetention {
	retention, interval := ResolveBillingLedgerRetention()
	worker := &BillingLedgerRetention{
		repo:      repo,
		retention: retention,
		interval:  interval,
		stop:      make(chan struct{}),
	}
	if repo == nil || retention <= 0 {
		return worker
	}
	if retention < 30*24*time.Hour {
		// Not an error: a window shorter than the audit retention simply means
		// almost nothing is ever deletable, because the audit row still exists.
		telemetry.ScopedLogger("billing").Warn(
			"billing ledger retention is very short; rows with a surviving audit row are never retired",
			"retention_days", int(retention.Hours()/24),
		)
	}
	worker.wg.Add(1)
	go worker.worker()
	return worker
}

// Stop waits for the worker to exit.
func (w *BillingLedgerRetention) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
}

func (w *BillingLedgerRetention) worker() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	select {
	case <-w.stop:
		return
	case <-time.After(billingLedgerPruneStartupDelay):
	}
	for {
		w.pruneOnce()
		select {
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

// pruneOnce runs one retention pass. It returns the number of retired ledger
// rows and the charge units moved into the archive aggregate.
func (w *BillingLedgerRetention) pruneOnce() (int64, int64, error) {
	if w == nil || w.repo == nil || w.repo.db == nil || w.retention <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().Add(-w.retention)

	var deleted, archivedUnits int64
	for batch := 0; batch < billingLedgerPruneMaxBatches; batch++ {
		batchRows, batchUnits, err := w.pruneBatchWithRetry(cutoff)
		if err != nil {
			return deleted, archivedUnits, err
		}
		deleted += batchRows
		archivedUnits += batchUnits
		if batchRows < billingLedgerPruneBatchSize {
			break
		}
	}

	if deleted > 0 {
		telemetry.ScopedLogger("billing").Warn(
			"retired expired billing ledger rows into the archive aggregate",
			"rows", deleted,
			"archived_units", archivedUnits,
			"archived_cny", FromUnits(archivedUnits),
			"retention_days", int(w.retention.Hours()/24),
			"cutoff", cutoff.UTC().Format(time.RFC3339),
			"archive_keys", billingLedgerArchiveKeyPrefix+"<yyyymm>",
		)
	}
	return deleted, archivedUnits, nil
}

// errBillingLedgerPruneContended marks a batch that had to roll back because
// another replica retired (or the ledger changed under us) some of the scanned
// rows. It must never be reported as "nothing to do": the earlier code returned
// (0, 0, nil), which made pruneOnce stop the whole pass silently and made a
// replica collision invisible in the logs.
var errBillingLedgerPruneContended = errors.New("billing ledger prune contended")

// ledgerPruneDeletionMismatch classifies a DELETE whose row count disagrees with
// the scanned candidate count.
func ledgerPruneDeletionMismatch(deleted, want int64) error {
	if deleted == want {
		return nil
	}
	return errBillingLedgerPruneContended
}

// pruneBatchWithRetry runs one batch and retries it once when a concurrent
// retention pass (another replica) raced it. The collision is reported instead of
// being folded into "nothing to do", so an operator can tell "the ledger has no
// expired rows" from "two replicas keep colliding".
func (w *BillingLedgerRetention) pruneBatchWithRetry(cutoff time.Time) (int64, int64, error) {
	rows, units, err := w.pruneBatch(cutoff)
	if !errors.Is(err, errBillingLedgerPruneContended) {
		return rows, units, err
	}
	telemetry.GlobalMetrics.BillingLedgerPruneContended.Add(1)
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-ledger-prune-contended", 10*time.Minute,
		"billing ledger retention batch collided with another replica; retrying it once",
		"cutoff", cutoff.UTC().Format(time.RFC3339),
	)
	rows, units, err = w.pruneBatch(cutoff)
	if errors.Is(err, errBillingLedgerPruneContended) {
		// Still contended: leave the rest of the pass to the next interval instead
		// of spinning, but keep the report honest.
		return 0, 0, err
	}
	return rows, units, err
}

// ledgerArchiveCandidate is one ledger row the retention policy is considering.
type ledgerArchiveCandidate struct {
	id        int64
	billingID string
	apiKey    string
	costUnits int64
	month     string
}

// pruneBatch retires up to billingLedgerPruneBatchSize rows in one transaction.
//
// The candidate scan, the "does the audit row still exist" check, the DELETE and
// the archive bookkeeping all run in the same transaction, so the amount added to
// the archive always corresponds to rows that really disappeared.
func (w *BillingLedgerRetention) pruneBatch(cutoff time.Time) (int64, int64, error) {
	cutoffValue := w.repo.dialectTimePrecise(cutoff)
	tx, err := w.repo.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	candidates, err := ledgerArchiveCandidates(tx, cutoffValue)
	if err != nil {
		return 0, 0, err
	}
	if len(candidates) == 0 {
		return 0, 0, nil
	}
	retirable, err := filterRetirableLedgerRows(tx, candidates)
	if err != nil {
		return 0, 0, err
	}
	if len(retirable) == 0 {
		// The oldest rows still have their audit evidence: nothing to do. Later
		// rows are younger, and age is what makes the evidence disappear, so
		// stopping here is correct and avoids rescanning the same rows.
		return 0, 0, nil
	}

	ids := make([]int64, 0, len(retirable))
	byKey := make(map[string]int64, len(retirable))
	byMonth := make(map[string]int64, len(retirable))
	var units int64
	for _, row := range retirable {
		ids = append(ids, row.id)
		units += row.costUnits
		byKey[row.apiKey] += row.costUnits
		byMonth[row.month] += row.costUnits
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	res, err := tx.Exec(`DELETE FROM billing_settlements WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if err := ledgerPruneDeletionMismatch(deleted, int64(len(ids))); err != nil {
		// Another replica's retention pass retired (or the ledger changed under
		// us) some of these rows between the scan and the DELETE. Rolling back is
		// the only safe answer: the archive must never claim units that are still
		// in the ledger. The caller reports the contention and retries.
		return 0, 0, err
	}
	if err := addLedgerArchiveUnits(tx, byKey); err != nil {
		return 0, 0, err
	}
	if err := addLedgerArchiveMonths(tx, byMonth); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return deleted, units, nil
}

// ledgerArchiveCandidates returns the oldest ledger rows older than cutoff.
//
// created_at IS NOT NULL is deliberate: a row whose timestamp is unknown cannot
// be proven old, so it is kept.
func ledgerArchiveCandidates(tx *Tx, cutoffValue any) ([]ledgerArchiveCandidate, error) {
	monthExpr := `COALESCE(strftime('%Y-%m', created_at), 'unknown')`
	if tx.db.Dialect() == "postgres" {
		monthExpr = `COALESCE(to_char(created_at, 'YYYY-MM'), 'unknown')`
	}
	query := `SELECT id, COALESCE(billing_id, ''), COALESCE(api_key, ''), COALESCE(cost_units, 0), ` + monthExpr + `
		FROM billing_settlements
		WHERE created_at IS NOT NULL AND created_at < ?
		ORDER BY created_at, id
		LIMIT ?`
	rows, err := tx.Query(query, cutoffValue, billingLedgerPruneBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []ledgerArchiveCandidate
	for rows.Next() {
		var row ledgerArchiveCandidate
		if err := rows.Scan(&row.id, &row.billingID, &row.apiKey, &row.costUnits, &row.month); err != nil {
			return nil, err
		}
		candidates = append(candidates, row)
	}
	return candidates, rows.Err()
}

// filterRetirableLedgerRows keeps only the candidates whose audit row no longer
// exists.
//
// Two shapes are checked, because both are real:
//   - modern rows: usage_logs.billing_id holds the same key (served by the unique
//     index uq_usage_billing_id);
//   - legacy rows: migration 23 recorded them as "legacy-<usage_logs.id>", so the
//     match is the audit row's primary key with an empty billing_id.
//
// A row whose audit row still exists is financial evidence and is never retired.
func filterRetirableLedgerRows(tx *Tx, candidates []ledgerArchiveCandidate) ([]ledgerArchiveCandidate, error) {
	modernIDs := make([]string, 0, len(candidates))
	legacyIDs := make([]int64, 0, len(candidates))
	for _, row := range candidates {
		billingID := strings.TrimSpace(row.billingID)
		if billingID == "" {
			// Cannot prove the audit row is gone; keep the row.
			continue
		}
		modernIDs = append(modernIDs, billingID)
		if id, ok := parseLegacyBillingID(billingID); ok {
			legacyIDs = append(legacyIDs, id)
		}
	}
	modern, err := existingUsageBillingIDs(tx, modernIDs)
	if err != nil {
		return nil, err
	}
	legacy, err := existingUsageLogIDs(tx, legacyIDs)
	if err != nil {
		return nil, err
	}

	retirable := make([]ledgerArchiveCandidate, 0, len(candidates))
	for _, row := range candidates {
		billingID := strings.TrimSpace(row.billingID)
		if billingID == "" {
			continue
		}
		if _, linked := modern[billingID]; linked {
			continue
		}
		if id, ok := parseLegacyBillingID(billingID); ok {
			if _, linked := legacy[id]; linked {
				continue
			}
		}
		retirable = append(retirable, row)
	}
	return retirable, nil
}

// parseLegacyBillingID recognizes the "legacy-<id>" key migration 23 assigned to
// audit rows that predate billing_id.
func parseLegacyBillingID(billingID string) (int64, bool) {
	if !strings.HasPrefix(billingID, legacyBillingIDPrefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(billingID, legacyBillingIDPrefix), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// existingUsageBillingIDs returns the subset of ids that some audit row still
// carries as its billing_id.
func existingUsageBillingIDs(tx *Tx, ids []string) (map[string]struct{}, error) {
	found := make(map[string]struct{}, len(ids))
	const chunk = 500
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := tx.Query(`SELECT billing_id FROM usage_logs WHERE billing_id IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			found[id] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return found, nil
}

// existingUsageLogIDs returns the subset of audit row ids that still exist with
// no billing_id (the legacy shape).
func existingUsageLogIDs(tx *Tx, ids []int64) (map[int64]struct{}, error) {
	found := make(map[int64]struct{}, len(ids))
	const chunk = 500
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := tx.Query(
			`SELECT id FROM usage_logs WHERE id IN (`+placeholders+`) AND COALESCE(billing_id, '') = ''`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			found[id] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return found, nil
}

// addLedgerArchiveUnits moves the retired units into the per-key aggregate.
//
// Keys are sorted and chunks are bounded so two replicas archiving at the same
// time take row locks in the same order (no deadlock) and stay far below the
// bound-parameter limit.
func addLedgerArchiveUnits(tx *Tx, byKey map[string]int64) error {
	keys := make([]string, 0, len(byKey))
	for key, units := range byKey {
		if units != 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	const chunk = 200
	for start := 0; start < len(keys); start += chunk {
		end := start + chunk
		if end > len(keys) {
			end = len(keys)
		}
		part := keys[start:end]
		placeholders := make([]string, len(part))
		args := make([]any, 0, len(part)*2)
		for i, key := range part {
			placeholders[i] = "(?, ?, CURRENT_TIMESTAMP)"
			args = append(args, key, byKey[key])
		}
		query := `INSERT INTO ` + billingLedgerArchivesTable + ` (api_key, cost_units, updated_at) VALUES ` +
			strings.Join(placeholders, ",") + `
			ON CONFLICT (api_key) DO UPDATE SET cost_units = COALESCE(cost_units, 0) + excluded.cost_units, updated_at = CURRENT_TIMESTAMP`
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

// addLedgerArchiveMonths records the retired amount under
// billing_ledger_archive_<yyyymm>, so the monthly archive total is readable
// without mining logs.
func addLedgerArchiveMonths(tx *Tx, byMonth map[string]int64) error {
	months := make([]string, 0, len(byMonth))
	for month, units := range byMonth {
		if units != 0 {
			months = append(months, month)
		}
	}
	sort.Strings(months)
	for _, month := range months {
		units := byMonth[month]
		// The addition happens in SQL so two replicas cannot lose an increment to a
		// read-modify-write race.
		if _, err := tx.Exec(`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(value AS BIGINT) + ? AS TEXT), updated_at = CURRENT_TIMESTAMP`,
			billingLedgerArchiveKeyPrefix+month, strconv.FormatInt(units, 10), units); err != nil {
			return err
		}
	}
	return nil
}
