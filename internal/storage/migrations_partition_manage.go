package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
