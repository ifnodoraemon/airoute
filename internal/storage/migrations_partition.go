package storage

// usage_logs monthly range partitioning (PostgreSQL only).
//
// Why partitioning: the audit table is the largest object in the gateway and its
// only removal path was batched DELETE, which leaves dead tuples, bloat and
// vacuum work behind. With monthly partitions the retention policy retires a whole
// month with one DROP TABLE (O(1), no dead tuples), and every console query that
// carries a created_at range probes 1–2 partitions instead of the whole history.
//
// Why this is opt-in: the copy-switch rewrites a live table. It takes an ACCESS
// EXCLUSIVE lock on usage_logs, copies every row (2× table size in WAL+heap), and
// rebuilds about a dozen indexes. Doing that automatically at first restart is
// exactly the kind of surprise a gateway must never spring on its operator, so
// migration 43 is only registered when GATEWAY_ENABLE_TABLE_PARTITIONING=true is
// set in the environment. With the flag unset (the default) no v43 migration is
// registered, nothing is rewritten, and startup is byte-for-byte the old one.
//
// Maintenance window: expect the audit write path to block for the duration of the
// copy (roughly one second per 10–20 MB of table on local SSD; a 250 MB table
// therefore blocks writes for tens of seconds and needs ~2× its size free for the
// new heap + WAL). Schedule it for a quiet period. The DDL runs under
// lock_timeout='5s', so if the exclusive lock cannot be taken the migration is
// deferred (errMigrationDeferred) instead of blocking startup; the next restart
// retries it.
//
// The hard constraint: PostgreSQL requires every UNIQUE/PRIMARY KEY on a
// partitioned table to contain the partition key, so `UNIQUE(billing_id)` cannot
// exist on the partitioned usage_logs. That unique index was the audit-row
// dedupe gate for at-least-once delivery. It is replaced by the short-lived
// registry table usage_audit_ids(billing_id PRIMARY KEY, created_at): the writer
// reserves the billing_id there first and only inserts the audit row when the
// reservation was new (same transaction). billing_settlements is deliberately NOT
// partitioned — it is the money-side idempotency gate, 29 MB, and its unique key
// must never be weakened.
//
// Deploy order matters. A replica still running a binary that predates this
// migration inserts with `ON CONFLICT (billing_id) DO NOTHING`, and a partitioned
// usage_logs has no matching unique index, so PostgreSQL rejects that statement
// with 42P10. Upgrade every replica to a build containing the registry writer path
// BEFORE enabling GATEWAY_ENABLE_TABLE_PARTITIONING, and keep the flag off during a
// rolling deploy. Charges are never at risk (the wallet settles before the audit
// row and the delivery is retried), but those replicas' audit writes would fail.
//
// Rollback: the switch keeps the pre-partition table as usage_logs_legacy_<unix>
// and never drops it, so reverting needs no dump. With the gateway stopped (no
// writers) and the flag unset:
//
//	BEGIN;
//	ALTER TABLE usage_logs RENAME TO usage_logs_partitioned_rollback;
//	ALTER TABLE usage_logs_legacy_<unix> RENAME TO usage_logs;
//	-- restore the canonical index names (they were prefixed with legacy_)
//	DO $$
//	DECLARE idx record;
//	BEGIN
//	  FOR idx IN SELECT indexname FROM pg_indexes
//	              WHERE schemaname = current_schema() AND tablename = 'usage_logs'
//	                AND indexname LIKE 'legacy\_%'
//	  LOOP
//	    EXECUTE format('ALTER INDEX %I RENAME TO %I', idx.indexname,
//	                   substr(idx.indexname, length('legacy_') + 1));
//	  END LOOP;
//	END $$;
//	ALTER SEQUENCE usage_logs_id_seq OWNED BY usage_logs.id;
//	COMMIT;
//	DELETE FROM schema_migrations WHERE version = 43;
//
// then restart with GATEWAY_ENABLE_TABLE_PARTITIONING unset. The partitioned copy can
// be dropped afterwards (DROP TABLE usage_logs_partitioned_rollback), together with
// usage_audit_ids once no partitioned writer remains.

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/lib/pq"
)

const (
	// enableTablePartitioningEnv gates migration 43. Only an explicit true enables
	// the high-risk table rewrite.
	enableTablePartitioningEnv = "GATEWAY_ENABLE_TABLE_PARTITIONING"

	// usageLogsPartitionedTable is the shadow table the copy lands in before the
	// name swap.
	usageLogsPartitionedTable = "usage_logs_partitioned"
	// usageLogsDefaultPartition catches rows whose created_at is outside every
	// provisioned month (NULL, clock skew, or months not created yet), so an audit
	// insert can never fail with "no partition of relation found".
	usageLogsDefaultPartition = "usage_logs_default"
	// usageLogsLegacyPrefix names the pre-partition table kept for rollback.
	usageLogsLegacyPrefix = "usage_logs_legacy_"

	// usageAuditIDsTable replaces UNIQUE(billing_id) as the audit dedupe gate on a
	// partitioned usage_logs.
	usageAuditIDsTable = "usage_audit_ids"
	// usageAuditIDRetentionDays is how long the registry must remember an id. It is
	// deliberately much shorter than the audit retention: after this window a
	// redelivery of the same billing_id would duplicate the audit row, which is
	// acceptable because redelivery windows (Redis PEL retention, spill replay) are
	// minutes-to-hours, while the registry would otherwise grow forever.
	usageAuditIDRetentionDays = 30

	// usageLogPartitionMonthsAhead is how many future months are pre-created on top
	// of the current one, so the writer keeps hitting a dedicated monthly partition.
	usageLogPartitionMonthsAhead = 6

	// usagePartitionLockTimeout bounds every DDL lock wait in migration 43 and in the
	// runtime partition provisioning. A DDL that cannot take its lock quickly is
	// deferred instead of freezing the gateway.
	usagePartitionLockTimeout = "5s"

	// usagePartitionLegacyIndexPrefix frees the canonical index names for the new
	// partitioned table while keeping the legacy indexes usable for rollback.
	usagePartitionLegacyIndexPrefix = "legacy_"
)

// usageLogPartitionBoundPattern extracts the range of a partition bound expression
// as rendered by pg_get_expr, e.g.
// FOR VALUES FROM ('2025-01-01 00:00:00+00') TO ('2025-02-01 00:00:00+00').
var usageLogPartitionBoundPattern = regexp.MustCompile(`FROM \('([^']+)'\) TO \('([^']+)'\)`)

// usageLogIndexDefPattern rewrites the source table name in a stored index
// definition (pg_indexes.indexdef) so it can be replayed against the shadow table.
var usageLogIndexDefPattern = regexp.MustCompile(`(?i)\bON\s+(?:[a-z_][a-z0-9_]*\.)?usage_logs\b`)

// tablePartitioningEnabled reports whether the operator explicitly opted into the
// usage_logs rewrite. Anything other than an affirmative value keeps the default:
// no v43 migration is registered at all.
func tablePartitioningEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(enableTablePartitioningEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// usageLogPartitionExecer is the subset of Exec shared by *sql.Tx and *DB/Tx, so
// partition DDL helpers work in both the migration transaction and the runtime
// provisioning transaction.
type usageLogPartitionExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// addUsageLogsMonthlyPartitions is migration 43.
//
// It is registered by migrations_postgres.go only when
// GATEWAY_ENABLE_TABLE_PARTITIONING is true; SQLite never registers it (SQLite has
// no declarative partitioning and keeps the single-table + UNIQUE(billing_id)
// path unchanged).
func addUsageLogsMonthlyPartitions(db *DB) error {
	if db == nil || db.Dialect() != "postgres" {
		// Defensive: the SQLite migration list must never include this migration.
		return nil
	}
	err := partitionUsageLogs(db)
	if err == nil {
		return nil
	}
	if errors.Is(err, errMigrationDeferred) {
		return err
	}
	if isPartitionLockConflict(err) {
		return fmt.Errorf("%w: usage_logs DDL could not acquire its lock within %s: %v",
			errMigrationDeferred, usagePartitionLockTimeout, err)
	}
	return err
}

// isPartitionLockConflict reports whether an error is a PostgreSQL transient
// lock/serialization failure that the migration runner should retry next start
// rather than treat as a schema bug.
func isPartitionLockConflict(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	switch string(pqErr.Code) {
	case "55P03", // lock_not_available (our lock_timeout fired)
		"57014", // query_canceled (statement_timeout / operator cancel)
		"40P01": // deadlock_detected
		return true
	}
	return false
}

// usageLogsRelkind returns the pg_class relkind of the live usage_logs table
// ("r" plain, "p" partitioned), or "" when the table does not exist.
func usageLogsRelkind(db *DB) (string, error) {
	var relkind string
	err := db.DB.QueryRow(`SELECT c.relkind
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = 'usage_logs' AND n.nspname = current_schema()`).Scan(&relkind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", err
	default:
		return relkind, nil
	}
}
