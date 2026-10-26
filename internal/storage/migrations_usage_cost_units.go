package storage

// backfillUsageLogCostUnits repairs audit rows written before the integer money
// mirror existed: they carry a float `cost` but `cost_units = 0`, so the integer
// ledger (and therefore reconciliation) under-counts them.
//
// On the live cluster this is 969 rows / ¥4.5002, which is most of the ¥7.15
// drift the reconciliation worker reported on its first run. The remainder
// (¥2.6474) belongs to rows that were deleted before ids were contiguous and
// cannot be reconstructed from the audit trail.
//
// The repair is deterministic (cost_units := round(cost * 1e9)) and only touches
// rows where the mirror is still zero, so re-running it is a no-op.
func backfillUsageLogCostUnits(db *DB) error {
	statement := `UPDATE usage_logs SET cost_units = CAST(ROUND(cost * 1000000000) AS BIGINT) WHERE cost_units = 0 AND cost <> 0`
	if db.Dialect() != "postgres" {
		statement = `UPDATE usage_logs SET cost_units = CAST(ROUND(cost * 1000000000) AS INTEGER) WHERE cost_units = 0 AND cost <> 0`
	}
	_, err := db.DB.Exec(statement)
	return err
}
