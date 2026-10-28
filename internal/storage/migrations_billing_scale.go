package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	// billingSettlementsAPIKeyIndex is the composite index the reconciliation
	// queries need.
	//
	// billing_settlements used to carry only (billing_id) and (created_at). Two
	// background queries walk it by api_key: ledgerDrift does
	// `GROUP BY api_key` over the whole table and needs the per-key charge total.
	// Without an api_key index that is a sequential scan plus a sort, and it grows
	// linearly with the ledger. With (api_key, cost_units) the aggregate becomes an
	// index-only scan in key order: no heap access, no sort, and the planner can
	// fold the SUM into the index.
	billingSettlementsAPIKeyIndex = "idx_billing_settlements_api_key"

	// billingLedgerArchivesTable keeps the money of ledger rows that the ledger
	// retention worker retired (see billing_ledger_retention.go).
	//
	// The ledger is a financial document: deleting a row must not change what the
	// reconciliation gauges report. Instead of dropping the amount, retention moves
	// it into this one-row-per-key aggregate. `auditLag` and `ledgerDrift` add it
	// back, so the book still balances after rows are archived, while the line-item
	// table stops growing forever.
	billingLedgerArchivesTable = "billing_ledger_archives"
)

// unusedUsageTrigramIndexes are the pg_trgm GIN indexes that cost real write
// amplification and are almost never used.
//
// The console keyword search rewrites its pattern to LIKE '%q%', which no B-tree
// can serve, so migration 19 added one trigram index per searchable column. The
// planner only uses them for that one-off search, and the production counters
// (pg_stat_user_indexes) showed 0-4 scans for five of the six indexes while they
// occupied ~91 MB of a 160 MB index footprint on usage_logs. Every audit insert
// pays for all of them. The error_message index is kept: it actually gets scanned
// (26 scans) and it backs the "why did this call fail" search, which is the one
// that needs to be fast during an incident.
var unusedUsageTrigramIndexes = []string{
	"idx_usage_trgm_chat_id",
	"idx_usage_trgm_trace_id",
	"idx_usage_trgm_session_id",
	"idx_usage_trgm_model",
	"idx_usage_trgm_channel",
}

// keptUsageTrigramIndex is documented so a later reader does not "finish the job"
// and drop the only trigram index that still earns its keep.
const keptUsageTrigramIndex = "idx_usage_trgm_error_message"

// addBillingScaleIndexes is migration 41: give reconciliation an index it can use
// and stop paying for unused audit indexes, plus create the aggregate the ledger
// retention policy needs.
//
// Atomicity: migrations_runner.go's applyMigration runs Up outside a transaction
// (only the version record follows it), so PostgreSQL is allowed to build and drop
// these indexes CONCURRENTLY. That matters on the live cluster: the plain form
// takes an ACCESS EXCLUSIVE lock for the whole build, which would block every
// settlement insert and audit write for the duration. SQLite has neither a
// CONCURRENTLY form nor a reader/writer concurrency model worth worrying about
// (the pool is a single writer), so it uses the plain statements.
//
// Still, a plain CREATE INDEX inside a transaction would be safe: if the runner
// ever starts wrapping migrations in one, PostgreSQL rejects CONCURRENTLY and the
// migration fails visibly instead of silently taking a heavy lock. The helper
// below therefore branches on the dialect instead of assuming one runner shape.
//
// The whole migration is idempotent: IF NOT EXISTS / IF EXISTS, a validity check
// for a half-built concurrent index, and CREATE TABLE IF NOT EXISTS. A replay
// after a crash between the DDL and the version record is a no-op.
func addBillingScaleIndexes(db *DB) error {
	// The archive aggregate is part of the reconciliation queries, so a failure to
	// create it is a real schema error and stops the upgrade.
	if err := createBillingLedgerArchivesTable(db); err != nil {
		return err
	}
	indexErr := createBillingSettlementsAPIKeyIndex(db)
	dropUnusedUsageTrigramIndexes(db)
	if indexErr != nil {
		// An index is a performance aid, not a correctness requirement, and a
		// failed CONCURRENTLY build (or a lock timeout on a busy primary) must not
		// keep the gateway down. Defer instead of failing: the runner will not
		// record version 41, log the reason, and retry on the next start — which is
		// safe because every statement above is idempotent.
		return fmt.Errorf("%w: %v", errMigrationDeferred, indexErr)
	}
	return nil
}

// createBillingLedgerArchivesTable creates the per-key aggregate that replaces
// retired ledger rows in the reconciliation totals.
func createBillingLedgerArchivesTable(db *DB) error {
	create := `CREATE TABLE IF NOT EXISTS ` + billingLedgerArchivesTable + ` (
		api_key VARCHAR(255) PRIMARY KEY,
		cost_units BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	)`
	if db.Dialect() != "postgres" {
		create = `CREATE TABLE IF NOT EXISTS ` + billingLedgerArchivesTable + ` (
			api_key TEXT PRIMARY KEY,
			cost_units INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`
	}
	if _, err := db.DB.Exec(create); err != nil {
		return fmt.Errorf("create %s: %w", billingLedgerArchivesTable, err)
	}
	return nil
}

// createBillingSettlementsAPIKeyIndex adds the reconciliation index, using
// CONCURRENTLY on PostgreSQL so the build never blocks settlement inserts.
func createBillingSettlementsAPIKeyIndex(db *DB) error {
	if db.Dialect() != "postgres" {
		if _, err := db.DB.Exec(
			`CREATE INDEX IF NOT EXISTS ` + billingSettlementsAPIKeyIndex + ` ON billing_settlements(api_key, cost_units)`); err != nil {
			return fmt.Errorf("create %s: %w", billingSettlementsAPIKeyIndex, err)
		}
		return nil
	}

	exists, valid, err := postgresIndexState(db, billingSettlementsAPIKeyIndex)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", billingSettlementsAPIKeyIndex, err)
	}
	if exists && valid {
		return nil
	}
	if exists && !valid {
		// A cancelled or failed CONCURRENTLY build leaves an INVALID index behind,
		// and `CREATE INDEX IF NOT EXISTS` would then match the name forever and
		// never repair it: the planner ignores invalid indexes, so the "index" would
		// be pure write overhead. Drop the corpse first.
		if _, err := db.DB.Exec(`DROP INDEX CONCURRENTLY IF EXISTS ` + billingSettlementsAPIKeyIndex); err != nil {
			return fmt.Errorf("drop invalid %s: %w", billingSettlementsAPIKeyIndex, err)
		}
	}
	if _, err := db.DB.Exec(
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS ` + billingSettlementsAPIKeyIndex + ` ON billing_settlements(api_key, cost_units)`); err != nil {
		return fmt.Errorf("create %s: %w", billingSettlementsAPIKeyIndex, err)
	}
	return nil
}

// postgresIndexState reports whether an index with this name exists in the
// current schema and whether the planner considers it valid.
func postgresIndexState(db *DB, name string) (exists bool, valid bool, err error) {
	query := `SELECT i.indisvalid
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = $1 AND n.nspname = current_schema()`
	switch err := db.DB.QueryRow(query, name).Scan(&valid); {
	case errors.Is(err, sql.ErrNoRows):
		return false, false, nil
	case err != nil:
		return false, false, err
	default:
		return true, valid, nil
	}
}

// dropUnusedUsageTrigramIndexes retires the audit trigram indexes that the
// planner does not use.
//
// Failures are logged, not returned: an index is a performance aid, and a
// leftover GIN index must not keep the gateway down. The next start retries
// because the version is only recorded when this migration returns nil.
func dropUnusedUsageTrigramIndexes(db *DB) {
	log := telemetry.ScopedLogger("system")
	for _, name := range unusedUsageTrigramIndexes {
		exists, err := storageIndexExists(db, name)
		if err != nil {
			log.Warn("could not inspect unused audit trigram index", "index", name, "error", err.Error())
			continue
		}
		if !exists {
			// Fresh database, or an earlier run already retired it. Do not log a drop
			// that did not happen: a boot-time log must stay trustworthy.
			continue
		}
		statement := `DROP INDEX IF EXISTS ` + name
		if db.Dialect() == "postgres" {
			// CONCURRENTLY keeps `usage_logs` writable while the drop waits for
			// in-flight transactions; the plain form takes an ACCESS EXCLUSIVE lock
			// on the busiest audit table in the gateway.
			statement = `DROP INDEX CONCURRENTLY IF EXISTS ` + name
		}
		if _, err := db.DB.Exec(statement); err != nil {
			log.Warn("could not drop unused audit trigram index", "index", name, "error", err.Error())
			continue
		}
		log.Info("dropped unused audit trigram index", "index", name)
	}
}

// indexExists reports whether a named index exists in the current schema.
func storageIndexExists(db *DB, name string) (bool, error) {
	if db.Dialect() == "postgres" {
		exists, _, err := postgresIndexState(db, name)
		return exists, err
	}
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
