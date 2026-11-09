package storage

import (
	"sort"
	"strings"
	"time"
)

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
