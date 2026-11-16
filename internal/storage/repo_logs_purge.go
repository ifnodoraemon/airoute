package storage

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// PurgeUsageLogsBefore deletes audit rows created before cutoff in bounded
// batches, so a large backlog cannot hold a long write lock on the data plane's
// database. Returns the number of deleted rows.
//
// On a partitioned usage_logs the whole expired months are retired first with
// DROP TABLE (no dead tuples, no vacuum work, O(1) instead of one DELETE per row);
// the bounded DELETE below then only has to handle the month that straddles the
// cutoff plus anything that landed in the DEFAULT partition. Both paths add the
// removed rows' charged units to the pruned-audit baseline in the same transaction,
// so audit-vs-ledger reconciliation cannot mistake deliberate retention for lost
// revenue.
func (r *Repository) PurgeUsageLogsBefore(cutoff time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 5000
	}
	var total int64
	if r.usageLogsPartitioned() {
		_, rowsDropped, err := r.dropExpiredUsageLogPartitions(cutoff)
		if err != nil {
			return 0, err
		}
		total += rowsDropped
	}
	cutoffValue := r.dialectTimePrecise(cutoff)
	for {
		tx, err := r.db.Begin()
		if err != nil {
			return total, err
		}
		// Retention may remove charged rows (they are old, but the money moved).
		// Their units are added to the pruned-audit baseline in the same
		// transaction, so the reconciliation gauge keeps measuring unexplained
		// differences instead of the whole pruned history.
		ids, units, err := usageLogsBefore(tx, cutoffValue, batchSize)
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if len(ids) == 0 {
			_ = tx.Rollback()
			break
		}
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args[i] = id
		}
		res, err := tx.Exec(`DELETE FROM usage_logs WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if err := addUsageAuditPrunedUnits(tx, units); err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += deleted
		if deleted < int64(batchSize) {
			break
		}
	}
	if total > 0 {
		// Pruned rows may have been the last use of a model or channel; the filter
		// dropdowns must not keep advertising them for a full cache TTL.
		r.invalidateUsageLogFacetCache()
	}
	return total, nil
}

// usageLogsBefore returns the ids and the charged units of the oldest rows that
// are older than cutoff, bounded by batchSize.
//
// The predicate is on the bare created_at column (the value is pre-formatted per
// dialect by the caller), which is what lets a partitioned usage_logs prune to the
// expired months and lets the id index serve the ORDER BY inside each of them.
func usageLogsBefore(tx *Tx, cutoffValue any, batchSize int) ([]int64, int64, error) {
	rows, err := tx.Query(`SELECT id, COALESCE(cost_units, 0) FROM usage_logs WHERE created_at < ? ORDER BY id LIMIT ?`,
		cutoffValue, batchSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []int64
	var units int64
	for rows.Next() {
		var id, rowUnits int64
		if err := rows.Scan(&id, &rowUnits); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
		units += rowUnits
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return ids, units, nil
}

func encodeThinkingObservation(observation *model.ThinkingObservation) string {
	if observation == nil {
		return ""
	}
	raw, _ := json.Marshal(observation)
	return string(raw)
}
