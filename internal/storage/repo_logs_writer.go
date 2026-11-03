package storage

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// usageLogsPartitionProbe caches, per database handle, whether usage_logs is the
// monthly-partitioned table.
//
// The answer only changes when migration 43 runs (or an operator restores a plain
// table), both of which happen at startup, so probing once per handle is enough and
// keeps the audit write path free of an extra catalog query per batch. The probe is
// keyed by *DB rather than stored on Repository so it also covers repositories built
// in tests without adding state to the shared struct.
var usageLogsPartitionProbe sync.Map // map[*DB]bool

// usageLogsPartitioned reports whether audit rows live in a partitioned usage_logs.
//
// Only PostgreSQL can return true: SQLite has no declarative partitioning and keeps
// the single-table + UNIQUE(billing_id) dedupe path.
func (r *Repository) usageLogsPartitioned() bool {
	if r == nil || r.db == nil {
		return false
	}
	if cached, ok := usageLogsPartitionProbe.Load(r.db); ok {
		return cached.(bool)
	}
	partitioned := false
	if r.db.Dialect() == "postgres" {
		var relkind string
		err := r.db.QueryRow(`SELECT c.relkind
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relname = 'usage_logs' AND n.nspname = current_schema()`).Scan(&relkind)
		partitioned = err == nil && relkind == "p"
	}
	usageLogsPartitionProbe.Store(r.db, partitioned)
	return partitioned
}

// invalidateUsageLogPartitionProbe drops the cached answer so the next audit write
// re-probes. Called by migration 43 after the copy-switch commits, and by tests
// that reshape the table behind the repository's back.
func invalidateUsageLogPartitionProbe(db *DB) {
	if db != nil {
		usageLogsPartitionProbe.Delete(db)
	}
}

// usageAuditIDReserveChunk bounds one multi-row registry INSERT (two parameters per
// row) well below PostgreSQL's 65535-parameter statement limit.
const usageAuditIDReserveChunk = 100

// reserveUsageAuditIDs reserves the batch's billing_ids in usage_audit_ids and
// returns only the records whose reservation was new.
//
// On a partitioned usage_logs there is no UNIQUE(billing_id) to conflict on —
// PostgreSQL requires every unique key to contain the partition key — so this
// registry's primary key becomes the single decision point for at-least-once
// delivery: the first delivery inserts the registry row and owns the audit row, and
// every redelivery's ON CONFLICT DO NOTHING returns nothing and is skipped. The
// reservation and the usage_logs insert share one transaction, so a failure rolls
// the reservation back and the redelivery can retry cleanly.
//
// billing_settlements deliberately stays unpartitioned: it is the money-side
// idempotency gate and must keep its own UNIQUE(billing_id).
func reserveUsageAuditIDs(tx *Tx, logs []*UsageLogRecord) ([]*UsageLogRecord, error) {
	// Collapse duplicates inside the batch first: one multi-row INSERT reserves at
	// most one row per id and would otherwise report the same id as new twice.
	seen := make(map[string]struct{}, len(logs))
	unique := make([]*UsageLogRecord, 0, len(logs))
	for _, log := range logs {
		if _, duplicate := seen[log.BillingID]; duplicate {
			continue
		}
		seen[log.BillingID] = struct{}{}
		unique = append(unique, log)
	}

	reserved := make(map[string]struct{}, len(unique))
	for start := 0; start < len(unique); start += usageAuditIDReserveChunk {
		end := start + usageAuditIDReserveChunk
		if end > len(unique) {
			end = len(unique)
		}
		values := make([]string, 0, end-start)
		args := make([]interface{}, 0, (end-start)*2)
		for _, log := range unique[start:end] {
			createdAt := log.CreatedAt
			if createdAt.IsZero() {
				createdAt = time.Now()
			}
			values = append(values, "(?, ?)")
			args = append(args, log.BillingID, createdAt.UTC())
		}
		query := `INSERT INTO ` + usageAuditIDsTable + ` (billing_id, created_at) VALUES ` +
			strings.Join(values, ",") + ` ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id`
		rows, err := tx.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var billingID string
			if err := rows.Scan(&billingID); err != nil {
				rows.Close()
				return nil, err
			}
			reserved[billingID] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	kept := make([]*UsageLogRecord, 0, len(reserved))
	for _, log := range unique {
		if _, fresh := reserved[log.BillingID]; !fresh {
			continue
		}
		kept = append(kept, log)
	}
	if skipped := len(logs) - len(kept); skipped > 0 {
		telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(uint64(skipped))
	}
	return kept, nil
}

// clampRunes returns s truncated to at most max runes ("" passes through).
func clampRunes(s string, max int) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// normalizeUsageLog clamps string fields to their storage column limits
// (model/channel/tenant_id VARCHAR(128), api_key VARCHAR(255) on Postgres).
func normalizeUsageLog(log *UsageLogRecord) {
	if log == nil {
		return
	}
	log.Model = clampRunes(log.Model, 128)
	log.Channel = clampRunes(log.Channel, 128)
	log.TenantID = clampRunes(log.TenantID, 128)
	log.APIKey = NormalizeStoredAPIKeyReference(log.APIKey)
	log.APIKey = clampRunes(log.APIKey, 255)
}

// NormalizeStoredAPIKeyReference turns any api_key column/JSON field value into the
// stored digest form.
//
// The request path already supplies the digest. The remaining sources are:
//
//   - a Redis Stream message or spill file written by a pre-migration binary, which
//     carries the cleartext credential. Hashing it yields exactly the digest the
//     migration wrote into api_keys.key for that credential, so settlement and
//     reconciliation still attribute the charge correctly - and no cleartext is
//     persisted;
//   - an auth-failure audit row (a key digest) or an operator-supplied label,
//     which hashes into a digest that matches no key and therefore behaves as an
//     unattributed charge, as before.
//
// Either way the column never holds a readable credential.
func NormalizeStoredAPIKeyReference(value string) string {
	if value == "" {
		return ""
	}
	if IsAPIKeyHash(value) {
		return value
	}
	return HashAPIKey(value)
}

// truncateError keeps persisted error strings bounded (2 KiB).
func truncateError(msg string) string {
	if len(msg) <= 2048 {
		return msg
	}
	cut := 2048
	for cut > 0 && (msg[cut]&0xC0) == 0x80 {
		cut--
	}
	return msg[:cut]
}

// normalizeUsageSource clamps the usage provenance marker to a known value.
//
// "gateway_auth" marks a client admission rejection (insufficient balance,
// disabled key, rate limit) recorded by the auth middleware. It used to be
// silently rewritten to "upstream", which made an operator denial look like an
// upstream-report row. Anything unknown still falls back to "upstream" so a typo
// can never invent a provenance the console cannot explain.
func normalizeUsageSource(src string) string {
	switch src {
	case "estimated", "gateway_auth":
		return src
	default:
		return "upstream"
	}
}

// RecordUsageLogContext is RecordUsageLog bound to a context/deadline.
//
// The charge is settled first, in its own transaction, and the audit row is
// written afterwards. Keeping them apart is what lets a charge survive an audit
// write failure: the money no longer depends on the log row.
func (r *Repository) RecordUsageLogContext(ctx context.Context, log *UsageLogRecord) error {
	normalizeUsageLog(log)
	if err := r.settleCharges(ctx, []*UsageLogRecord{log}); err != nil {
		return err
	}
	return r.insertUsageLogsContext(ctx, []*UsageLogRecord{log})
}

// BatchRecordUsageLogsContext is BatchRecordUsageLogs bound to a context.
// Background writers must use a deadline: without one a stalled database pins
// the flush goroutine forever, the in-memory queue fills up, and request
// completion starts blocking on backpressure.
func (r *Repository) BatchRecordUsageLogsContext(ctx context.Context, logs []*UsageLogRecord) error {
	if len(logs) == 0 {
		return nil
	}
	for _, log := range logs {
		normalizeUsageLog(log)
	}
	// 1. Money first: idempotent per billing_id and independent of the audit row.
	if err := r.settleCharges(ctx, logs); err != nil {
		return err
	}
	// 2. Audit rows second: also idempotent per billing_id; a failure is retried
	//    by the caller (Redis PEL, spill replay, in-memory retry).
	return r.insertUsageLogsContext(ctx, logs)
}

// insertUsageLogsContext persists audit rows only. It never moves money; that is
// settleCharges' job.
func (r *Repository) insertUsageLogsContext(ctx context.Context, logs []*UsageLogRecord) error {
	filtered := make([]*UsageLogRecord, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		if log.BillingID == "" {
			log.BillingID = NewBillingID()
		}
		filtered = append(filtered, log)
	}
	if len(filtered) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Partitioned mode has no UNIQUE(billing_id) to carry the dedupe, so reserve the
	// ids in usage_audit_ids first and insert only the rows whose reservation was
	// new. Both statements are in this transaction, so the registry and the audit
	// row are all-or-nothing.
	partitioned := r.usageLogsPartitioned()
	if partitioned {
		filtered, err = reserveUsageAuditIDs(tx, filtered)
		if err != nil {
			return err
		}
		if len(filtered) == 0 {
			return tx.Commit()
		}
	}

	const (
		insertColumns  = `trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_units, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, error_message, pricing_snapshot, created_at, billing_id`
		insertColumnsN = 23
		insertChunk    = 25
	)

	placeholdersFor := func(rows int) string {
		rowPlaceholder := "(" + strings.Repeat("?, ", insertColumnsN-1) + "?)"
		placeholders := make([]string, rows)
		for i := range placeholders {
			placeholders[i] = rowPlaceholder
		}
		return strings.Join(placeholders, ",")
	}

	appendArgs := func(args []interface{}, log *UsageLogRecord) []interface{} {
		isOff := 0
		if log.IsOffPeak {
			isOff = 1
		}
		discount := log.OffPeakDiscount
		if discount <= 0 {
			discount = 1.0
		}
		// A distinct fallback per row; borrowing the session id would collapse a
		// whole conversation onto one trace id.
		ensureTraceID(log)
		createdAt := log.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		// Every column is supplied so rows in one VALUES list share a shape, and
		// created_at always carries microseconds.
		return append(args,
			log.TraceID, log.ChatID, log.SessionID, log.APIKey, log.TenantID, log.Model, log.Channel,
			log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToUnits(log.Cost),
			isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, normalizeUsageSource(log.UsageSource),
			truncateError(log.ErrorMessage), encodePricingSnapshot(log.PricingSnapshot),
			r.dialectTimePrecise(createdAt), log.BillingID,
		)
	}

	if len(filtered) == 1 {
		log := filtered[0]
		query := `INSERT INTO usage_logs (` + insertColumns + `) VALUES ` + placeholdersFor(1)
		if !partitioned {
			// The single-table schema dedupes with the unique index; the partitioned
			// one already deduped in usage_audit_ids above.
			query += ` ON CONFLICT (billing_id) DO NOTHING`
		}
		res, err := tx.Exec(query, appendArgs(make([]interface{}, 0, insertColumnsN), log)...)
		if err != nil {
			return err
		}
		if !partitioned {
			if affected, raErr := res.RowsAffected(); raErr == nil && affected == 0 {
				telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(1)
			}
		}
		return tx.Commit()
	}

	rowArgs := make([]interface{}, 0, len(filtered)*insertColumnsN)
	for _, log := range filtered {
		rowArgs = appendArgs(rowArgs, log)
	}

	rowCount := len(rowArgs) / insertColumnsN
	inserted := 0
	for start := 0; start < rowCount; start += insertChunk {
		end := start + insertChunk
		if end > rowCount {
			end = rowCount
		}
		query := `INSERT INTO usage_logs (` + insertColumns + `) VALUES ` + placeholdersFor(end-start)
		if partitioned {
			// Every row here already holds a fresh usage_audit_ids reservation, so a
			// plain insert is exactly-once and there is no unique index to conflict
			// with (ON CONFLICT (billing_id) would be a SQL error on the partitioned
			// table).
			if _, err := tx.Exec(query, rowArgs[start*insertColumnsN:end*insertColumnsN]...); err != nil {
				return err
			}
			inserted += end - start
			continue
		}
		query += ` ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id`
		rows, err := tx.Query(query, rowArgs[start*insertColumnsN:end*insertColumnsN]...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var billingID string
			if err := rows.Scan(&billingID); err != nil {
				rows.Close()
				return err
			}
			inserted++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if skipped := rowCount - inserted; skipped > 0 {
		telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(uint64(skipped))
	}
	return tx.Commit()
}
