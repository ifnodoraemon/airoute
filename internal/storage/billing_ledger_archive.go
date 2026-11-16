package storage

import (
	"sort"
	"strconv"
	"strings"
)

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
		// PostgreSQL resolves an unqualified cost_units ambiguously between the
		// target row and excluded (SQLSTATE 42702); qualify it like the sibling
		// addUsageAuditPrunedUnits. SQLite only accepts the bare column name.
		currentUnits := "cost_units"
		if tx != nil && tx.db != nil && tx.db.Dialect() == "postgres" {
			currentUnits = billingLedgerArchivesTable + ".cost_units"
		}
		query := `INSERT INTO ` + billingLedgerArchivesTable + ` (api_key, cost_units, updated_at) VALUES ` +
			strings.Join(placeholders, ",") + `
			ON CONFLICT (api_key) DO UPDATE SET cost_units = COALESCE(` + currentUnits + `, 0) + excluded.cost_units, updated_at = CURRENT_TIMESTAMP`
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
		// read-modify-write race. Qualify value for PostgreSQL (SQLSTATE 42702),
		// matching addUsageAuditPrunedUnits; SQLite uses the bare column name.
		currentValue := "value"
		if tx != nil && tx.db != nil && tx.db.Dialect() == "postgres" {
			currentValue = "system_settings.value"
		}
		if _, err := tx.Exec(`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(`+currentValue+` AS BIGINT) + ? AS TEXT), updated_at = CURRENT_TIMESTAMP`,
			billingLedgerArchiveKeyPrefix+month, strconv.FormatInt(units, 10), units); err != nil {
			return err
		}
	}
	return nil
}
