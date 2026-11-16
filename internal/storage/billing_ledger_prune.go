package storage

import (
	"errors"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
