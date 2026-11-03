package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const usageLogErrorSQL = `(COALESCE(status_code, 0) < 200 OR status_code >= 400 OR COALESCE(error_message, '') <> '')`
const usageLogSuccessSQL = `(status_code >= 200 AND status_code < 400 AND COALESCE(error_message, '') = '')`

// MaxUsageLogPageSize is the largest page the API will serve. The admin handler
// clamps to the same value so the page size it echoes back is the one the
// repository actually applied; letting the two drift made clients page by a
// size they never received.
const MaxUsageLogPageSize = 200

// escapeLikePattern makes user input match literally inside a LIKE pattern.
// Without it a keyword such as "100%" or "a_b" silently acts as a wildcard.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// sqliteTimestampLayout is how usage_logs.created_at is written (UTC, fixed
// width). Comparing against that exact format is what lets SQLite use
// idx_usage_created.
//
// Microseconds are mandatory. The previous whole-second layout collapsed every
// row of a flush onto the same instant: a burst of 502s all read 02:09:44 with
// no way to order them, measure retry spacing, or correlate them with the
// nanosecond-precision access log. It also made the audit page mix two
// timestamp shapes ("...:44" and "...:44.905597") in one page.
const sqliteTimestampLayout = "2006-01-02 15:04:05.000000"

// legacySecondTimestampLayout is how rows written before the precision fix were
// stored (SQLite only; PostgreSQL kept whatever precision it was given).
// Time filters still accept it so old rows remain queryable.
const legacySecondTimestampLayout = "2006-01-02 15:04:05"

// ftsPhrase renders user text as an FTS5 phrase, doubling embedded quotes so the
// match grammar cannot interpret it as operators.
func ftsPhrase(keyword string) string {
	return `"` + strings.ReplaceAll(keyword, `"`, `""`) + `"`
}

// sqliteUsageLogFTSAvailable reports whether migration v20 created the FTS5 index.
func (r *Repository) sqliteUsageLogFTSAvailable() bool {
	r.ftsOnce.Do(func() {
		if r.db.Dialect() != "sqlite" {
			return
		}
		var count int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'usage_logs_fts'`).Scan(&count); err == nil {
			r.ftsAvailable = count > 0
		}
	})
	return r.ftsAvailable
}

// normalizeSQLiteTimestamp converts an API time filter (RFC3339 from the console,
// or a plain date) into the stored representation.
func normalizeSQLiteTimestamp(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, layout := range []string{
		sqliteTimestampLayout,
		legacySecondTimestampLayout,
		"2006-01-02T15:04:05",
		time.RFC3339Nano,
		"2006-01-02",
	} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC().Format(sqliteTimestampLayout), true
		}
	}
	return "", false
}

// timeFilterClause builds one side of the created_at range. SQLite stores the
// column as a UTC string, so normalizing the parameter lets the bare column be
// compared; wrapping it in datetime() forced a full scan on every page load.
// Values that cannot be parsed keep the legacy wrapped form rather than silently
// matching nothing.
func (r *Repository) timeFilterClause(op, raw string) (string, interface{}) {
	if raw == "" {
		return "", nil
	}
	if r.db.Dialect() == "postgres" {
		return ` AND created_at ` + op + ` ?`, raw
	}
	if normalized, ok := normalizeSQLiteTimestamp(raw); ok {
		return ` AND created_at ` + op + ` ?`, normalized
	}
	return ` AND datetime(created_at) ` + op + ` datetime(?)`, raw
}

// UsagePricingSnapshot is the rate actually applied when a request was billed.
// Historical charges must never be explained using today's editable prices.
type UsagePricingSnapshot struct {
	Model           string  `json:"model"`
	GroupName       string  `json:"group_name"`
	PriceGroup      string  `json:"price_group"`
	Currency        string  `json:"currency"`
	PromptPrice     float64 `json:"prompt_price"`
	CompletionPrice float64 `json:"completion_price"`
	CacheReadPrice  float64 `json:"cache_read_price"`
	// CacheWritePrice is the rate applied to prompt-cache *writes*; providers
	// charge a premium over the plain input rate for them.
	CacheWritePrice float64 `json:"cache_write_price,omitempty"`
	// CacheCreationTokens is how many of PromptTokens were written to the cache.
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	FixedPrice          float64 `json:"fixed_price"`
	GroupMultiplier     float64 `json:"group_multiplier"`
	OffPeakDiscount     float64 `json:"off_peak_discount"`
	BillableUnits       int     `json:"billable_units"`
}

func encodePricingSnapshot(snapshot *UsagePricingSnapshot) string {
	if snapshot == nil {
		return ""
	}
	encoded, _ := json.Marshal(snapshot)
	return string(encoded)
}

// UsageLogRecord represents an audit log entry.
type UsageLogRecord struct {
	ID               int64                 `json:"id"`
	TraceID          string                `json:"trace_id"`
	ChatID           string                `json:"chat_id,omitempty"`
	SessionID        string                `json:"session_id,omitempty"`
	APIKey           string                `json:"api_key"`
	TenantID         string                `json:"tenant_id"`
	Model            string                `json:"model"`
	Channel          string                `json:"channel"`
	PromptTokens     int                   `json:"prompt_tokens"`
	CompletionTokens int                   `json:"completion_tokens"`
	CachedTokens     int                   `json:"cached_tokens"`
	TotalTokens      int                   `json:"total_tokens"`
	Cost             float64               `json:"cost"`
	PricingSnapshot  *UsagePricingSnapshot `json:"pricing_snapshot,omitempty"`
	IsOffPeak        bool                  `json:"is_off_peak"`
	OffPeakDiscount  float64               `json:"off_peak_discount"`
	DurationMs       int64                 `json:"duration_ms"`
	TTFTMs           int64                 `json:"ttft_ms"`
	StatusCode       int                   `json:"status_code"`
	UsageSource      string                `json:"usage_source"` // "upstream" (reported) or "estimated" (local fallback)
	ErrorMessage     string                `json:"error_message,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	// BillingID is the server-generated idempotency key for this billable row.
	// It is never taken from client input: trace_id is client-controllable
	// (X-Request-ID / traceparent) and one reused value already spans dozens of
	// genuine requests, so a unique index on it would break billing. A retried
	// Redis stream delivery carries the same BillingID and is applied once.
	BillingID string `json:"billing_id,omitempty"`
	// Settled marks a record whose wallet movement has already been applied
	// synchronously. The async writer skips the settlement for those rows; the
	// settlement ledger's unique key is the actual guarantee, this is only an
	// optimisation that avoids a redundant conflict probe.
	Settled bool `json:"settled,omitempty"`
}

// LogFilter defines search/filtering criteria for usage audit logs.
type LogFilter struct {
	Limit          int
	Offset         int
	StartTime      string // e.g. "2026-09-26 00:00:00" or ISO8601
	EndTime        string
	TraceID        string
	ChatID         string
	SessionID      string
	Model          string
	TenantID       string
	APIKeys        []string
	ScopeByAPIKeys bool
	APIKeyID       int64
	// Channel narrows to one upstream channel name.
	Channel string
	// Status is "success" | "error" | "" (all).
	Status string
	// Cache is "cached" | "uncached" | "" (all).
	Cache string
	// Latency is "fast" (<=1000ms) | "slow" (>1000ms) | "" (all), measured on duration_ms.
	Latency string
	// Keyword is a free-text match across trace/chat/session/model/channel/error.
	Keyword string
	// SkipCount omits the total-row COUNT. Dashboard polls and CSV export only
	// need the rows, and COUNT over the filtered set is the expensive part of a
	// page request.
	SkipCount bool
}

// ListUsageLogsWithFilter queries logs with flexible filters (time range, trace ID, session ID, model, tenant).
// buildUsageLogWhere assembles the WHERE clause and args shared by the page
// query and the total-count query, so pagination always reflects the complete
// filtered result set.
func (r *Repository) buildUsageLogWhere(f LogFilter) (string, []interface{}) {
	where := ""
	var args []interface{}

	if clause, value := r.timeFilterClause(">=", f.StartTime); clause != "" {
		where += clause
		args = append(args, value)
	}
	if clause, value := r.timeFilterClause("<=", f.EndTime); clause != "" {
		where += clause
		args = append(args, value)
	}
	// These three filters are deliberately left unindexed.
	//
	// Each one is an equality OR a leading-wildcard LIKE on the same column. A
	// B-tree cannot serve `LIKE '%q%'`, and the OR means both sides must be
	// evaluated, so an index on the column is never chosen — it only costs a write
	// on every audit insert (the hottest write path). Migration 42 dropped the
	// single-column indexes v1/v7/v10 created for them. The tenant/key/model
	// filters, which are equality plus a created_at range, keep their composites.
	if f.TraceID != "" {
		where += ` AND (trace_id = ? OR trace_id LIKE ? ESCAPE '\')`
		args = append(args, f.TraceID, "%"+escapeLikePattern(f.TraceID)+"%")
	}
	if f.ChatID != "" {
		where += ` AND (chat_id = ? OR chat_id LIKE ? ESCAPE '\')`
		args = append(args, f.ChatID, "%"+escapeLikePattern(f.ChatID)+"%")
	}
	if f.SessionID != "" {
		where += ` AND (session_id = ? OR session_id LIKE ? ESCAPE '\')`
		args = append(args, f.SessionID, "%"+escapeLikePattern(f.SessionID)+"%")
	}
	if f.Model != "" {
		where += ` AND model = ?`
		args = append(args, f.Model)
	}
	if f.TenantID != "" {
		where += ` AND tenant_id = ?`
		args = append(args, f.TenantID)
	}
	if len(f.APIKeys) > 0 {
		placeholders := make([]string, 0, len(f.APIKeys))
		for _, k := range f.APIKeys {
			// Callers may pass the stored digest (the console resolves key ids to
			// api_keys.key) or a bare identifier; both normalize to the digest form
			// that the api_key column holds.
			k = NormalizeStoredAPIKeyReference(k)
			placeholders = append(placeholders, "?")
			args = append(args, k)
		}
		where += ` AND api_key IN (` + strings.Join(placeholders, ",") + `)`
	}
	if f.APIKeyID > 0 {
		where += ` AND api_key IN (SELECT key FROM api_keys WHERE id = ?)`
		args = append(args, f.APIKeyID)
	}
	if f.Channel != "" {
		where += ` AND channel = ?`
		args = append(args, f.Channel)
	}
	if f.Status == "success" {
		where += ` AND ` + usageLogSuccessSQL
	} else if f.Status == "error" {
		where += ` AND ` + usageLogErrorSQL
	}
	if f.Cache == "cached" {
		where += ` AND COALESCE(cached_tokens, 0) > 0`
	} else if f.Cache == "uncached" {
		where += ` AND COALESCE(cached_tokens, 0) = 0`
	}
	if f.Latency == "fast" {
		where += ` AND COALESCE(duration_ms, 0) <= 1000`
	} else if f.Latency == "slow" {
		where += ` AND COALESCE(duration_ms, 0) > 1000`
	}
	if f.Keyword != "" {
		// Free-text search must run in SQL: filtering only the loaded page
		// silently misses every match living on another page.
		like := "%" + escapeLikePattern(f.Keyword) + "%"
		likeClause := ` AND (trace_id LIKE ? ESCAPE '\' OR COALESCE(chat_id,'') LIKE ? ESCAPE '\' OR COALESCE(session_id,'') LIKE ? ESCAPE '\' OR COALESCE(model,'') LIKE ? ESCAPE '\' OR COALESCE(channel,'') LIKE ? ESCAPE '\' OR COALESCE(error_message,'') LIKE ? ESCAPE '\')`
		// The trigram tokenizer cannot answer queries shorter than three
		// characters, so those keep the LIKE scan; everything else goes through
		// the FTS5 index instead of scanning six columns.
		if r.db.Dialect() == "sqlite" && utf8.RuneCountInString(f.Keyword) >= 3 && r.sqliteUsageLogFTSAvailable() {
			where += ` AND id IN (SELECT rowid FROM usage_logs_fts WHERE usage_logs_fts MATCH ?)`
			args = append(args, ftsPhrase(f.Keyword))
		} else {
			where += likeClause
			args = append(args, like, like, like, like, like, like)
		}
	}
	return where, args
}

// ListUsageLogsWithFilter queries logs with flexible filters (time range, trace ID, session ID, model, tenant).
func (r *Repository) ListUsageLogsWithFilter(f LogFilter) ([]*UsageLogRecord, error) {
	logs, _, err := r.ListUsageLogsWithFilterAndCount(f)
	return logs, err
}

// ListUsageLogsWithFilterAndCount also returns the total number of rows matching
// the filter (ignoring LIMIT/OFFSET), so the UI can paginate the real result set.
//
// Partitioning note: the COUNT and the page query share buildUsageLogWhere, and every
// time filter it emits is a plain `created_at >= / <= ?` comparison (no wrapper
// function on either dialect), so on a partitioned usage_logs both statements prune
// to the months the request touches and neither changes meaning. A request that
// supplies no time range legitimately spans the whole retained history and therefore
// probes every partition — exactly the work the old single-table scan did — so the
// console should keep sending a range for large deployments.
func (r *Repository) ListUsageLogsWithFilterAndCount(f LogFilter) ([]*UsageLogRecord, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > MaxUsageLogPageSize {
		f.Limit = MaxUsageLogPageSize
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.ScopeByAPIKeys && len(f.APIKeys) == 0 {
		return make([]*UsageLogRecord, 0), 0, nil
	}

	where, whereArgs := r.buildUsageLogWhere(f)

	// -1 means "not counted"; callers that asked to skip it must not use the value.
	total := int64(-1)
	if !f.SkipCount {
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE 1=1`+where, whereArgs...).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	query := `SELECT id, trace_id, COALESCE(chat_id, ''), COALESCE(session_id, ''), COALESCE(api_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost, 0.0), COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, COALESCE(usage_source, 'upstream'), COALESCE(error_message, ''), COALESCE(pricing_snapshot, ''), created_at FROM usage_logs WHERE 1=1` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args := append(append([]interface{}{}, whereArgs...), f.Limit, f.Offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*UsageLogRecord
	for rows.Next() {
		var l UsageLogRecord
		var isOff int
		var createdAt time.Time
		var pricingJSON string
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ChatID, &l.SessionID, &l.APIKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &l.UsageSource, &l.ErrorMessage, &pricingJSON, &createdAt); err != nil {
			return nil, 0, err
		}
		if pricingJSON != "" {
			_ = json.Unmarshal([]byte(pricingJSON), &l.PricingSnapshot)
		}
		l.IsOffPeak = isOff == 1
		l.CreatedAt = createdAt
		logs = append(logs, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ErrChargedUsageLogImmutable is returned when an operator tries to delete an
// audit row that carries a charge.
//
// A charged row is the evidence for money that already left a wallet, and the
// settlement ledger (billing_settlements) records that charge permanently. Hard
// deleting the row did not reverse anything: it only made the charge
// unexplainable, and it pushed the audit-vs-ledger reconciliation permanently out
// of balance. Corrections have to go through a reversal, not a delete.
var ErrChargedUsageLogImmutable = errors.New("该调用日志已产生计费，不能删除；请通过冲正流程更正")

// usagePrunedUnitsSetting is the running total of charge units whose audit row was
// deliberately removed. Reconciliation adds it back, so pruning does not look
// like lost revenue forever.
const usagePrunedUnitsSetting = "usage_log_pruned_cost_units"

// addUsageAuditPrunedUnits adds units to the pruned-audit baseline inside tx.
//
// The addition happens in SQL so two replicas pruning at the same time cannot
// lose an increment to a read-modify-write race.
func addUsageAuditPrunedUnits(tx *Tx, units int64) error {
	if units <= 0 {
		return nil
	}
	// PostgreSQL requires the target side of ON CONFLICT DO UPDATE to be qualified:
	// an unqualified `value` is ambiguous between the stored row and the proposed
	// row (SQLSTATE 42702), which made every PostgreSQL retention pass fail. SQLite
	// only accepts a bare column name there, so the two dialects need different
	// spellings of the same expression.
	currentValue := "value"
	if tx != nil && tx.db != nil && tx.db.Dialect() == "postgres" {
		currentValue = "system_settings.value"
	}
	_, err := tx.Exec(`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(`+currentValue+` AS BIGINT) + ? AS TEXT), updated_at = CURRENT_TIMESTAMP`,
		usagePrunedUnitsSetting, strconv.FormatInt(units, 10), units)
	return err
}

// UsageAuditPrunedUnits returns the charge units whose audit rows were removed on
// purpose (retention pruning). Reconciliation subtracts them from the expected
// ledger total.
func (r *Repository) UsageAuditPrunedUnits() int64 {
	raw := strings.TrimSpace(r.GetSetting(usagePrunedUnitsSetting, "0"))
	units, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return units
}

// chargedUsageLogSQL matches an audit row that carries money. Legacy rows written
// before cost_units existed still have a float cost, so both mirrors are checked.
const chargedUsageLogSQL = `(COALESCE(cost, 0) <> 0 OR COALESCE(cost_units, 0) <> 0)`

// DeleteUsageLog deletes a single audit log by ID.
//
// Only a row that was never charged may be removed; a charged row is billing
// evidence and returns ErrChargedUsageLogImmutable.
func (r *Repository) DeleteUsageLog(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var charged bool
	switch err := tx.QueryRow(`SELECT CASE WHEN `+chargedUsageLogSQL+` THEN 1 ELSE 0 END FROM usage_logs WHERE id = ?`, id).Scan(&charged); {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	case charged:
		return ErrChargedUsageLogImmutable
	}

	if _, err := tx.Exec(`DELETE FROM usage_logs WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.invalidateUsageLogFacetCache()
	return nil
}

// BatchDeleteUsageLogs deletes multiple usage logs by their IDs and reports how
// many were kept because they carry a charge.
func (r *Repository) BatchDeleteUsageLogs(ids []int64) (deleted int64, skipped int64, err error) {
	if len(ids) == 0 {
		return 0, 0, nil
	}
	inList, args := inClause[int64](nil, ids)

	tx, err := r.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE id IN (`+inList+`) AND `+chargedUsageLogSQL, args...).
		Scan(&skipped); err != nil {
		return 0, 0, err
	}
	res, err := tx.Exec(`DELETE FROM usage_logs WHERE id IN (`+inList+`) AND NOT `+chargedUsageLogSQL, args...)
	if err != nil {
		return 0, 0, err
	}
	if deleted, err = res.RowsAffected(); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	if deleted > 0 {
		r.invalidateUsageLogFacetCache()
	}
	return deleted, skipped, nil
}

// ClearAllUsageLogs wipes every audit row that carries no charge and reports how
// many charged rows were kept (they are billing evidence, see
// ErrChargedUsageLogImmutable).
func (r *Repository) ClearAllUsageLogs() (deleted int64, skipped int64, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE ` + chargedUsageLogSQL).Scan(&skipped); err != nil {
		return 0, 0, err
	}
	res, err := tx.Exec(`DELETE FROM usage_logs WHERE NOT ` + chargedUsageLogSQL)
	if err != nil {
		return 0, 0, err
	}
	if deleted, err = res.RowsAffected(); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	// The dropdowns are cached; an operator who just cleared the trail must not
	// keep seeing model/channel options for another TTL window.
	r.invalidateUsageLogFacetCache()
	return deleted, skipped, nil
}

// PurgeUsageLogsBefore deletes audit rows created before cutoff in bounded
// batches, so a large backlog cannot hold a long write lock on the data plane's
// database. Returns the number of deleted rows.
//
// On a partitioned usage_logs the whole expired months are retired first with
// DROP TABLE (no dead tuples, no vacuum work, O(1) instead of one DELETE per row);
// the bounded DELETE below then only has to handle the month that straddles the
// cutoff plus anything that landed in the DEFAULT partition. Both paths add the
// removed rows' charged units to the pruned-audit baseline in the same transaction,
// so audit-vs-ledger reconciliation cannot mistake deliberate retention for lost
// revenue.
func (r *Repository) PurgeUsageLogsBefore(cutoff time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 5000
	}
	var total int64
	if r.usageLogsPartitioned() {
		_, rowsDropped, err := r.dropExpiredUsageLogPartitions(cutoff)
		if err != nil {
			return 0, err
		}
		total += rowsDropped
	}
	cutoffValue := r.dialectTimePrecise(cutoff)
	for {
		tx, err := r.db.Begin()
		if err != nil {
			return total, err
		}
		// Retention may remove charged rows (they are old, but the money moved).
		// Their units are added to the pruned-audit baseline in the same
		// transaction, so the reconciliation gauge keeps measuring unexplained
		// differences instead of the whole pruned history.
		ids, units, err := usageLogsBefore(tx, cutoffValue, batchSize)
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if len(ids) == 0 {
			_ = tx.Rollback()
			break
		}
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args[i] = id
		}
		res, err := tx.Exec(`DELETE FROM usage_logs WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if err := addUsageAuditPrunedUnits(tx, units); err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += deleted
		if deleted < int64(batchSize) {
			break
		}
	}
	if total > 0 {
		// Pruned rows may have been the last use of a model or channel; the filter
		// dropdowns must not keep advertising them for a full cache TTL.
		r.invalidateUsageLogFacetCache()
	}
	return total, nil
}

// usageLogsBefore returns the ids and the charged units of the oldest rows that
// are older than cutoff, bounded by batchSize.
//
// The predicate is on the bare created_at column (the value is pre-formatted per
// dialect by the caller), which is what lets a partitioned usage_logs prune to the
// expired months and lets the id index serve the ORDER BY inside each of them.
func usageLogsBefore(tx *Tx, cutoffValue any, batchSize int) ([]int64, int64, error) {
	rows, err := tx.Query(`SELECT id, COALESCE(cost_units, 0) FROM usage_logs WHERE created_at < ? ORDER BY id LIMIT ?`,
		cutoffValue, batchSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []int64
	var units int64
	for rows.Next() {
		var id, rowUnits int64
		if err := rows.Scan(&id, &rowUnits); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
		units += rowUnits
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return ids, units, nil
}
