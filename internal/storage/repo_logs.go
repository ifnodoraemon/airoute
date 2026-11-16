package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const usageLogErrorSQL = `(COALESCE(status_code, 0) < 200 OR status_code >= 400 OR COALESCE(error_message, '') <> '')`
const usageLogSuccessSQL = `(status_code >= 200 AND status_code < 400 AND COALESCE(error_message, '') = '')`

// MaxUsageLogPageSize is the largest page the API will serve. The admin handler
// clamps to the same value so the page size it echoes back is the one the
// repository actually applied; letting the two drift made clients page by a
// size they never received.
const MaxUsageLogPageSize = 200

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

	query := `SELECT id, trace_id, COALESCE(chat_id, ''), COALESCE(session_id, ''), COALESCE(api_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost_units, 0) / 1000000000.0, COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, COALESCE(usage_source, 'upstream'), COALESCE(error_message, ''), COALESCE(pricing_snapshot, ''), created_at, COALESCE(thinking, ''), COALESCE(channel_trail, '') FROM usage_logs WHERE 1=1` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
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
		var pricingJSON, thinkingJSON string
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ChatID, &l.SessionID, &l.APIKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &l.UsageSource, &l.ErrorMessage, &pricingJSON, &createdAt, &thinkingJSON, &l.ChannelTrail); err != nil {
			return nil, 0, err
		}
		if pricingJSON != "" {
			_ = json.Unmarshal([]byte(pricingJSON), &l.PricingSnapshot)
		}
		if thinkingJSON != "" {
			_ = json.Unmarshal([]byte(thinkingJSON), &l.Thinking)
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

// chargedUsageLogSQL matches an audit row that carries an integer ledger charge.
const chargedUsageLogSQL = `(COALESCE(cost_units, 0) <> 0)`

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
