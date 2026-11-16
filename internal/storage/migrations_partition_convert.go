package storage

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
