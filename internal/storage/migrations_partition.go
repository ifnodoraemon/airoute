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
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
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

// partitionUsageLogs performs the copy-switch inside one transaction.
//
// Sequence of operations (all atomic; a failure leaves the live table untouched):
//
//  1. crate the shadow partitioned parent + monthly children + DEFAULT partition;
//  2. capture the old index definitions, rename the old indexes out of the way,
//     and rebuild them on the (still empty) shadow table so the expensive builds
//     happen outside the exclusive lock;
//  3. create the usage_audit_ids registry;
//  4. LOCK TABLE usage_logs IN ACCESS EXCLUSIVE MODE (the maintenance-window gate,
//     bounded by lock_timeout);
//  5. copy every row, backfill the registry for the redelivery window, and verify
//     the row count matches before touching any name;
//  6. rename usage_logs → usage_logs_legacy_<unix> and
//     usage_logs_partitioned → usage_logs, then hand the id sequence over;
//  7. commit.
//
// Idempotency: a committed switch without a recorded schema_migrations row (crash
// between commit and the version insert) is detected by relkind='p' and treated as
// done. A leftover shadow table from an interrupted manual attempt is dropped after
// a WARN; the live data is never touched.
func partitionUsageLogs(db *DB) error {
	log := telemetry.ScopedLogger("system")

	relkind, err := usageLogsRelkind(db)
	if err != nil {
		return err
	}
	switch relkind {
	case "p":
		// Already partitioned: either a previous run committed the switch but crashed
		// before recording the version, or the operator re-enabled the flag. Make sure
		// the registry exists and let the runner record the version.
		if err := ensureUsageAuditIDsTable(db.DB); err != nil {
			return err
		}
		invalidateUsageLogPartitionProbe(db)
		log.Info("usage_logs is already partitioned; skipping the copy-switch")
		return nil
	case "":
		return fmt.Errorf("usage_logs does not exist; cannot partition it")
	}

	legacyTables, err := legacyUsageLogTables(db)
	if err != nil {
		return err
	}
	if len(legacyTables) > 0 {
		// A previous switch (or a manual rollback) left a legacy table behind. It is
		// not the live table, so continuing is safe and loses nothing; the operator
		// should drop it once the rollback window has passed.
		log.Warn("a previous usage_logs legacy table already exists; the switch will keep both",
			"legacy_tables", strings.Join(legacyTables, ","))
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Protect the DDL from waiting forever behind the data plane, and lift the
	// per-statement timeout for the one long INSERT ... SELECT.
	for _, stmt := range []string{
		`SET LOCAL lock_timeout = '` + usagePartitionLockTimeout + `'`,
		`SET LOCAL statement_timeout = 0`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	// A shadow table can only be left over by an interrupted manual attempt: this
	// function's own transaction is all-or-nothing. It is not live, so dropping it
	// cannot lose data.
	var shadowExists bool
	if err := tx.QueryRow(`SELECT EXISTS (
			SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE c.relname = $1 AND n.nspname = current_schema())`, usageLogsPartitionedTable).Scan(&shadowExists); err != nil {
		return err
	}
	if shadowExists {
		log.Warn("dropping a leftover usage_logs_partitioned table before retrying the switch")
		if _, err := tx.Exec(`DROP TABLE ` + usageLogsPartitionedTable); err != nil {
			return err
		}
	}

	now := time.Now().UTC()

	// 1. Shadow parent + partitions.
	if _, err := tx.Exec(`CREATE TABLE ` + usageLogsPartitionedTable + ` (
		LIKE usage_logs INCLUDING DEFAULTS INCLUDING STORAGE
	) PARTITION BY RANGE (created_at)`); err != nil {
		return fmt.Errorf("create %s: %w", usageLogsPartitionedTable, err)
	}
	firstMonth, lastMonth, err := usageLogPartitionMonthRange(tx, now)
	if err != nil {
		return err
	}
	for month := firstMonth; !month.After(lastMonth); month = month.AddDate(0, 1, 0) {
		if err := createUsageLogMonthPartition(tx, usageLogsPartitionedTable, month); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE TABLE ` + usageLogsDefaultPartition + ` PARTITION OF ` +
		usageLogsPartitionedTable + ` DEFAULT`); err != nil {
		return fmt.Errorf("create %s: %w", usageLogsDefaultPartition, err)
	}

	// 2. Move the old indexes out of the way, then rebuild them on the empty shadow
	// table. Building indexes before the exclusive lock keeps the write-blocking
	// window down to the copy itself (plus the catalog renames).
	oldIndexes, err := usageLogIndexDefinitions(tx)
	if err != nil {
		return err
	}
	for _, index := range oldIndexes {
		if strings.HasPrefix(index.name, usagePartitionLegacyIndexPrefix) {
			continue
		}
		legacyName := truncatePGIdentifier(usagePartitionLegacyIndexPrefix + index.name)
		if _, err := tx.Exec(`ALTER INDEX ` + quotePGIdentifier(index.name) + ` RENAME TO ` + quotePGIdentifier(legacyName)); err != nil {
			return fmt.Errorf("rename legacy index %s: %w", index.name, err)
		}
	}
	for _, index := range oldIndexes {
		switch {
		case index.name == "usage_logs_pkey":
			// A primary key on the partitioned parent would have to include
			// created_at, and a plain id PRIMARY KEY cannot exist. The id sequence
			// still makes ids unique; the audit dedupe gate now lives in
			// usage_audit_ids. Skip it rather than weaken the shape of the new table.
			continue
		case index.name == "uq_usage_billing_id":
			// Replaced by usage_audit_ids: UNIQUE(billing_id) is impossible on a
			// partitioned table (the partition key must be part of every unique key),
			// and a non-unique index on billing_id (created below) covers lookups.
			continue
		case strings.Contains(strings.ToUpper(index.definition), " UNIQUE INDEX "):
			// Any other unique index would be rejected by PostgreSQL unless it
			// contains created_at. Warn instead of failing the whole switch.
			log.Warn("not rebuilding a unique index on the partitioned audit table",
				"index", index.name, "reason", "unique indexes must include the partition key")
			continue
		}
		definition := usageLogIndexDefPattern.ReplaceAllString(index.definition, " ON "+usageLogsPartitionedTable)
		if _, err := tx.Exec(definition); err != nil {
			return fmt.Errorf("rebuild index %s: %w", index.name, err)
		}
	}
	// New indexes the partitioned table needs regardless of the legacy set: id keeps
	// ORDER BY id DESC / delete-by-id efficient across partitions, billing_id keeps
	// the by-id lookups (ledger retention, gap repair) from scanning every partition.
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_usage_id ON ` + usageLogsPartitionedTable + ` (id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_billing_id ON ` + usageLogsPartitionedTable + ` (billing_id)`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	// 3. Dedupe registry (replaces UNIQUE(billing_id)).
	if err := ensureUsageAuditIDsTable(tx); err != nil {
		return err
	}

	// 4. The maintenance-window gate: from here until commit every audit write
	// waits. lock_timeout turns a busy database into a deferred migration instead
	// of a stalled startup.
	if _, err := tx.Exec(`LOCK TABLE usage_logs IN ACCESS EXCLUSIVE MODE`); err != nil {
		return err
	}

	// 5. Copy. `LIKE` preserves column order, so SELECT * lines up exactly.
	if _, err := tx.Exec(`INSERT INTO ` + usageLogsPartitionedTable + ` SELECT * FROM usage_logs`); err != nil {
		return fmt.Errorf("copy usage_logs rows: %w", err)
	}
	// Backfill the registry for the redelivery window only: older ids are outside
	// the registry's retention anyway, and a full-table backfill would needlessly
	// lengthen the exclusive lock.
	registryCutoff := now.AddDate(0, 0, -usageAuditIDRetentionDays)
	if _, err := tx.Exec(`INSERT INTO `+usageAuditIDsTable+` (billing_id, created_at)
		SELECT billing_id, created_at FROM usage_logs
		 WHERE billing_id IS NOT NULL AND billing_id <> '' AND created_at >= $1
		ON CONFLICT (billing_id) DO NOTHING`, registryCutoff); err != nil {
		return fmt.Errorf("backfill %s: %w", usageAuditIDsTable, err)
	}

	// Verify before renaming: a mismatch means the copy lost or duplicated rows, and
	// the transaction must abort while the live table is still untouched.
	var oldRows, newRows int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&oldRows); err != nil {
		return err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ` + usageLogsPartitionedTable).Scan(&newRows); err != nil {
		return err
	}
	if oldRows != newRows {
		return fmt.Errorf("partition copy verification failed: usage_logs has %d rows, %s has %d",
			oldRows, usageLogsPartitionedTable, newRows)
	}

	// 6. Switch names. The old table is kept for rollback, never dropped.
	legacyTable := usageLogsLegacyPrefix + strconv.FormatInt(now.Unix(), 10)
	seqName := ""
	var serialSeq sql.NullString
	if err := tx.QueryRow(`SELECT pg_get_serial_sequence('usage_logs', 'id')`).Scan(&serialSeq); err != nil {
		return err
	}
	if serialSeq.Valid {
		seqName = serialSeq.String
	}
	for _, stmt := range []string{
		`ALTER TABLE usage_logs RENAME TO ` + quotePGIdentifier(legacyTable),
		`ALTER TABLE ` + usageLogsPartitionedTable + ` RENAME TO usage_logs`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if seqName != "" {
		// Explicit ids were copied, so the sequence must move past them or the next
		// insert would reuse an existing id. Re-owning it to the new parent also keeps
		// it alive when the operator later drops the legacy table.
		var maxID, lastValue int64
		if err := tx.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM usage_logs`).Scan(&maxID); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT last_value FROM ` + seqName).Scan(&lastValue); err != nil {
			return err
		}
		if lastValue > maxID {
			maxID = lastValue
		}
		if _, err := tx.Exec(`ALTER SEQUENCE ` + seqName + ` OWNED BY usage_logs.id`); err != nil {
			return err
		}
		if _, err := tx.Exec(`SELECT setval('` + seqName + `', ` + strconv.FormatInt(maxID, 10) + `)`); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	invalidateUsageLogPartitionProbe(db)
	log.Info("usage_logs partitioned by month",
		"legacy_table", legacyTable,
		"rows", oldRows,
		"first_month", firstMonth.Format("2006-01"),
		"last_month", lastMonth.Format("2006-01"))
	return nil
}

// usageLogPartitionMonthRange returns the first and last month that need a
// partition: the month of the oldest row through the later of "current month +
// usageLogPartitionMonthsAhead" and "newest row month".
func usageLogPartitionMonthRange(tx *sql.Tx, now time.Time) (time.Time, time.Time, error) {
	var minCreated, maxCreated sql.NullTime
	if err := tx.QueryRow(`SELECT MIN(created_at), MAX(created_at) FROM usage_logs`).Scan(&minCreated, &maxCreated); err != nil {
		return time.Time{}, time.Time{}, err
	}
	first := usageLogMonthStart(now)
	if minCreated.Valid {
		first = usageLogMonthStart(minCreated.Time.UTC())
	}
	last := usageLogMonthStart(now).AddDate(0, usageLogPartitionMonthsAhead, 0)
	if maxCreated.Valid {
		if newest := usageLogMonthStart(maxCreated.Time.UTC()); newest.After(last) {
			last = newest
		}
	}
	return first, last, nil
}

// usageLogMonthStart truncates t to the first instant of its UTC month.
func usageLogMonthStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// usageLogPartitionName is the monthly partition name for a month start.
func usageLogPartitionName(monthStart time.Time) string {
	return "usage_logs_" + monthStart.UTC().Format("2006_01")
}

// createUsageLogMonthPartition creates one monthly partition if it does not exist.
func createUsageLogMonthPartition(execer usageLogPartitionExecer, parent string, monthStart time.Time) error {
	from := monthStart.UTC()
	to := from.AddDate(0, 1, 0)
	const layout = "2006-01-02 15:04:05-07"
	stmt := `CREATE TABLE IF NOT EXISTS ` + quotePGIdentifier(usageLogPartitionName(from)) +
		` PARTITION OF ` + quotePGIdentifier(parent) +
		` FOR VALUES FROM ('` + from.Format(layout) + `') TO ('` + to.Format(layout) + `')`
	if _, err := execer.Exec(stmt); err != nil {
		return fmt.Errorf("create partition %s: %w", usageLogPartitionName(from), err)
	}
	return nil
}

// ensureUsageAuditIDsTable creates the audit dedupe registry and its retention
// index. It is safe to call from a migration transaction and from a runtime
// connection, and is idempotent.
func ensureUsageAuditIDsTable(execer usageLogPartitionExecer) error {
	if _, err := execer.Exec(`CREATE TABLE IF NOT EXISTS ` + usageAuditIDsTable + ` (
		billing_id TEXT PRIMARY KEY,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create %s: %w", usageAuditIDsTable, err)
	}
	if _, err := execer.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_audit_ids_created ON ` +
		usageAuditIDsTable + ` (created_at)`); err != nil {
		return fmt.Errorf("index %s.created_at: %w", usageAuditIDsTable, err)
	}
	return nil
}

// usageLogIndex is one row of pg_indexes for the pre-partition table.
type usageLogIndex struct {
	name       string
	definition string
}

// usageLogIndexDefinitions returns the index name/definition pairs of the live
// usage_logs table, which the switch replays against the partitioned table.
func usageLogIndexDefinitions(tx *sql.Tx) ([]usageLogIndex, error) {
	rows, err := tx.Query(`SELECT indexname, indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename = 'usage_logs'
		ORDER BY indexname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []usageLogIndex
	for rows.Next() {
		var index usageLogIndex
		if err := rows.Scan(&index.name, &index.definition); err != nil {
			return nil, err
		}
		indexes = append(indexes, index)
	}
	return indexes, rows.Err()
}

// legacyUsageLogTables lists the usage_logs_legacy_* tables, if any.
func legacyUsageLogTables(db *DB) ([]string, error) {
	rows, err := db.DB.Query(`SELECT c.relname
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r'
		  AND c.relname LIKE '` + usageLogsLegacyPrefix + `%'
		ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// usageLogPartition describes one child partition of a partitioned usage_logs.
type usageLogPartition struct {
	name      string
	to        time.Time
	isDefault bool
}

// usageLogPartitions lists the live partitions of usage_logs (empty when the table
// is not partitioned). It is used by retention (to drop whole expired months) and
// by provisioning (to skip months that already exist).
func (r *Repository) usageLogPartitions() ([]usageLogPartition, error) {
	if r == nil || r.db == nil || r.db.Dialect() != "postgres" {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT c.relname, pg_get_expr(c.relpartbound, c.oid)
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		JOIN pg_namespace n ON n.oid = p.relnamespace
		WHERE p.relname = 'usage_logs' AND n.nspname = current_schema()
		ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var partitions []usageLogPartition
	for rows.Next() {
		var name, bound string
		if err := rows.Scan(&name, &bound); err != nil {
			return nil, err
		}
		partition := usageLogPartition{name: name}
		if strings.EqualFold(strings.TrimSpace(bound), "DEFAULT") {
			partition.isDefault = true
			partitions = append(partitions, partition)
			continue
		}
		match := usageLogPartitionBoundPattern.FindStringSubmatch(bound)
		if match == nil {
			// An unexpected bound shape must not make retention delete the wrong
			// partition: skip it and leave it to the batched DELETE path.
			continue
		}
		_, fromErr := parsePartitionBoundTime(match[1])
		to, toErr := parsePartitionBoundTime(match[2])
		if fromErr != nil || toErr != nil {
			continue
		}
		partition.to = to
		partitions = append(partitions, partition)
	}
	return partitions, rows.Err()
}

// parsePartitionBoundTime parses the timestamp rendering pg_get_expr uses for a
// partition bound.
func parsePartitionBoundTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05.999999-07",
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised partition bound %q", raw)
}

// EnsureUsageLogMonthPartitions provisions the current month and the configured
// number of future months. It is a no-op unless usage_logs is partitioned.
//
// The writer already has a DEFAULT partition as a safety net, but rows landing
// there lose partition pruning and cannot be retired with a DROP, so the retention
// worker keeps the calendar ahead of the clock. Adding a partition takes a brief
// ACCESS EXCLUSIVE lock on the parent, hence the bounded lock_timeout and the
// "log and retry next cycle" contract with the caller.
func (r *Repository) EnsureUsageLogMonthPartitions(monthsAhead int) error {
	if r == nil || r.db == nil || !r.usageLogsPartitioned() {
		return nil
	}
	if monthsAhead < 0 {
		monthsAhead = usageLogPartitionMonthsAhead
	}
	existing := make(map[string]struct{})
	partitions, err := r.usageLogPartitions()
	if err != nil {
		return err
	}
	for _, partition := range partitions {
		existing[partition.name] = struct{}{}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`SET LOCAL lock_timeout = '` + usagePartitionLockTimeout + `'`); err != nil {
		return err
	}
	first := usageLogMonthStart(time.Now())
	last := first.AddDate(0, monthsAhead, 0)
	for month := first; !month.After(last); month = month.AddDate(0, 1, 0) {
		if _, ok := existing[usageLogPartitionName(month)]; ok {
			continue
		}
		if err := createUsageLogMonthPartition(tx, "usage_logs", month); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// dropExpiredUsageLogPartitions drops every monthly partition whose whole range is
// older than cutoff and returns how many partitions (and rows) it retired. Rows of
// the one month that straddles the cutoff are left to the batched DELETE path.
//
// The retired rows' charged units are added to the pruned-audit baseline in the
// same transaction as the DROP, exactly like the batched DELETE path does, so the
// reconciliation gauge keeps measuring unexplained gaps instead of the whole
// pruned history.
func (r *Repository) dropExpiredUsageLogPartitions(cutoff time.Time) (partitionsDropped int, rowsDropped int64, err error) {
	partitions, err := r.usageLogPartitions()
	if err != nil {
		return 0, 0, err
	}
	for _, partition := range partitions {
		if partition.isDefault || partition.to.IsZero() || partition.to.After(cutoff) {
			continue
		}
		tx, err := r.db.Begin()
		if err != nil {
			return partitionsDropped, rowsDropped, err
		}
		var units, rows int64
		if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(cost_units), 0) FROM `+quotePGIdentifier(partition.name)).Scan(&rows, &units); err != nil {
			_ = tx.Rollback()
			return partitionsDropped, rowsDropped, fmt.Errorf("inspect %s before drop: %w", partition.name, err)
		}
		if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + quotePGIdentifier(partition.name)); err != nil {
			_ = tx.Rollback()
			return partitionsDropped, rowsDropped, fmt.Errorf("drop partition %s: %w", partition.name, err)
		}
		if err := addUsageAuditPrunedUnits(tx, units); err != nil {
			_ = tx.Rollback()
			return partitionsDropped, rowsDropped, err
		}
		if err := tx.Commit(); err != nil {
			return partitionsDropped, rowsDropped, err
		}
		partitionsDropped++
		rowsDropped += rows
		telemetry.ScopedLogger("billing").Info("dropped expired usage_logs partition",
			"partition", partition.name, "rows", rows, "cost_units", units)
	}
	return partitionsDropped, rowsDropped, nil
}

// quotePGIdentifier double-quotes a PostgreSQL identifier.
func quotePGIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// truncatePGIdentifier keeps generated identifiers within PostgreSQL's 63-byte
// limit.
func truncatePGIdentifier(name string) string {
	if len(name) <= 63 {
		return name
	}
	return name[:63]
}
