package storage

import "fmt"

// This file reconstructs facts that earlier binaries failed to record, and it
// writes off one historical counter artefact. Every statement is idempotent: the
// migration runner may replay a migration when the process dies between the
// update and the version record, and the same SQL is also applied to the live
// cluster by hand.
const (
	// legacyNilWrapReason replaces the "%!w(<nil>)" artefact that
	// fmt.Errorf("...%w", nil) wrote into the audit trail. The wrapped error was
	// nil because the dispatch loop only skipped channels (all of them circuit
	// broken), which is exactly the condition skippedOnlyError describes.
	legacyNilWrapReason = "every candidate channel was temporarily disabled by the circuit breaker, so no upstream was called (reconstructed: the original row was recorded through a log-formatting bug)"

	// legacySynthetic499Reason replaces an empty error_message on a synthesized
	// 499. The gateway, not the provider, produces 499 when the downstream goes
	// away; the pre-release binary recorded the status without a reason. Some of
	// these rows carry a charge for tokens the provider had already produced, so
	// the text deliberately makes no claim about billing.
	legacySynthetic499Reason = "client closed request: 499 is synthesized by the gateway for a downstream disconnect and is not a status code returned by the upstream provider (legacy row: the reason was not recorded by the pre-release binary)"

	// f9DeletedAuditKeyID and f9DeletedAuditArtifactUnits identify the single
	// historical write-off described in docs/audit/billing.md (finding F9).
	//
	// 147 audit rows (ids 1-147) were deleted by the pre-guard delete endpoints
	// before they refused to remove charged rows. The row counters kept the
	// charge, so api_keys.used_cost_units stayed exactly ¥2.647492 above the
	// settlement ledger for that key. Both numbers below are a fingerprint: if
	// they do not both match, this migration does nothing at all, which is what
	// keeps it safe on any other deployment.
	f9DeletedAuditKeyID         = int64(1)
	f9DeletedAuditArtifactUnits = int64(2647492000) // 1e-9 CNY units => ¥2.647492
	f9DeletedAuditWriteOffKey   = "billing_repair_f9_writeoff"
)

// f9DeletedAuditArtifactCNY is the float mirror of f9DeletedAuditArtifactUnits.
// settlement.go increments used_cost (float) and used_cost_units together, so a
// repair has to move both or the key reads inconsistently in the console.
func f9DeletedAuditArtifactCNY() float64 {
	return float64(f9DeletedAuditArtifactUnits) / 1e9
}

// tableHasColumn reports whether the table carries column.
//
// A database that predates migration 14 has no error_message column, and a
// replay of the version table against a hand-built legacy shape can miss it too.
// Callers use this to degrade to a no-op instead of aborting an upgrade.
func tableHasColumn(db *DB, table, column string) bool {
	if db.Dialect() == "postgres" {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM information_schema.columns
			 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			table, column,
		).Scan(&n); err != nil {
			return false
		}
		return n > 0
	}
	return sqliteColumnExists(db.DB, table, column)
}

// rewriteLegacyUsageLogReasons reconstructs the error_message of audit rows
// written by binaries that lost the reason: the fmt %w-with-nil artefact and the
// synthetic 499 with no text.
//
// The rows themselves and their status codes are history and are not rewritten:
// a client really did receive that status. Only the explanation changes, and it
// says that it was reconstructed.
func rewriteLegacyUsageLogReasons(db *DB) error {
	if !tableHasColumn(db, "usage_logs", "error_message") {
		return nil
	}
	if _, err := db.Exec(
		`UPDATE usage_logs SET error_message = REPLACE(error_message, '%!w(<nil>)', ?) WHERE error_message LIKE '%!w(<nil>)%'`,
		legacyNilWrapReason,
	); err != nil {
		return fmt.Errorf("rewrite nil-wrapped dispatch errors: %w", err)
	}
	if _, err := db.Exec(
		`UPDATE usage_logs SET error_message = ? WHERE status_code = 499 AND COALESCE(error_message, '') = ''`,
		legacySynthetic499Reason,
	); err != nil {
		return fmt.Errorf("backfill synthetic 499 reasons: %w", err)
	}
	return nil
}

// repairDeletedAuditCounterDrift writes off the F9 counter artefact.
//
// It subtracts exactly the known amount rather than recomputing the counter from
// the ledger: settlements keep accruing while the statement runs, and an
// absolute assignment read from a snapshot would drop a concurrent charge. The
// subtraction is also re-runnable, because after the first pass the guard no
// longer matches.
//
// The removed money is recorded in system_settings so the write-off stays
// explainable to anyone reading the key later. used_tokens is deliberately left
// alone: the tokens were really consumed, only the audit rows were deleted, and
// there is no ledger to derive their count from.
func repairDeletedAuditCounterDrift(db *DB) error {
	res, err := db.Exec(
		`UPDATE api_keys
		 SET used_cost_units = COALESCE(used_cost_units, 0) - ?,
		     used_cost = COALESCE(used_cost, 0) - ?
		 WHERE id = ?
		   AND COALESCE(used_cost_units, 0) - COALESCE((SELECT SUM(cost_units) FROM billing_settlements WHERE api_key = api_keys.key), 0) = ?`,
		f9DeletedAuditArtifactUnits, f9DeletedAuditArtifactCNY(), f9DeletedAuditKeyID, f9DeletedAuditArtifactUnits,
	)
	if err != nil {
		return fmt.Errorf("write off deleted-audit counter drift: %w", err)
	}
	repaired, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("write off deleted-audit counter drift: %w", err)
	}
	if repaired == 0 {
		return nil
	}

	note := fmt.Sprintf(
		"key_id=%d units=%d cny=%.6f origin=F9 deleted audit rows ids 1-147; counter written down to the settlement ledger",
		f9DeletedAuditKeyID, f9DeletedAuditArtifactUnits, f9DeletedAuditArtifactCNY(),
	)
	if _, err := db.Exec(
		`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP`,
		f9DeletedAuditWriteOffKey, note, note,
	); err != nil {
		return fmt.Errorf("record deleted-audit write-off: %w", err)
	}
	return nil
}
