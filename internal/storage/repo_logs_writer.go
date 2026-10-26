package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
	log.APIKey = clampRunes(log.APIKey, 255)
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
func normalizeUsageSource(src string) string {
	if src == "estimated" {
		return "estimated"
	}
	return "upstream"
}

// RecordUsageLog records an audit log and updates key quota/cost atomically.
func (r *Repository) RecordUsageLog(log *UsageLogRecord) error {
	return r.RecordUsageLogContext(context.Background(), log)
}

// RecordUsageLogContext is RecordUsageLog bound to a context/deadline.
func (r *Repository) RecordUsageLogContext(ctx context.Context, log *UsageLogRecord) error {
	normalizeUsageLog(log)
	isOff := 0
	if log.IsOffPeak {
		isOff = 1
	}
	discount := log.OffPeakDiscount
	if discount <= 0 {
		discount = 1.0
	}
	if log.TraceID == "" {
		if log.SessionID != "" {
			log.TraceID = log.SessionID
		} else {
			log.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixMicro())
		}
	}

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	keyVal := log.APIKey
	usageSource := normalizeUsageSource(log.UsageSource)

	// The idempotency key must exist before the insert so a redelivery of this
	// same record (Redis PEL replay / AutoClaim) collides instead of charging
	// the wallet twice.
	billingID := log.BillingID
	if billingID == "" {
		billingID = NewBillingID()
		log.BillingID = billingID
	}

	// Always write created_at ourselves at microsecond precision. Leaving it to
	// the column default produced whole-second SQLite timestamps, and formatting
	// it with a second-precision layout collapsed an entire flush onto one
	// instant — the two paths together are why one audit page mixed
	// "...:44" with "...:44.905597".
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	createdAtValue := createdAt.UTC().Format(sqliteTimestampLayout)

	var res sql.Result
	res, err = tx.Exec(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_units, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, error_message, pricing_snapshot, created_at, billing_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (billing_id) DO NOTHING`,
		log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToUnits(log.Cost), isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, usageSource, truncateError(log.ErrorMessage), encodePricingSnapshot(log.PricingSnapshot), createdAtValue, billingID)
	if err != nil {
		return err
	}
	// Zero rows affected means this record was already persisted by an earlier
	// delivery whose XACK was lost. The wallet and key counters were updated in
	// that same transaction, so committing now must not move money again.
	if affected, raErr := res.RowsAffected(); raErr == nil && affected == 0 {
		telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(1)
		return tx.Commit()
	}

	if keyVal != "" && (log.Cost > 0 || log.TotalTokens > 0) {
		if _, err := tx.Exec(r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_units = COALESCE(used_cost_units, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`),
			log.Cost, ToUnits(log.Cost), log.TotalTokens, keyVal); err != nil {
			return err
		}
		if log.Cost > 0 {
			applied, _, err := r.deductWallet(tx,
				`id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`, []any{keyVal}, log.Cost)
			if err != nil {
				return err
			}
			if !applied && log.TenantID != "" {
				var isOwnerAdmin bool
				row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), keyVal)
				if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
					// Admin keys are exempt from balance deduction; do not fallback to tenant
				} else {
					if _, _, err := r.deductWallet(tx,
						`(username = ? OR email = ?) AND role != 'admin'`, []any{log.TenantID, log.TenantID}, log.Cost); err != nil {
						return err
					}
				}
			}
		}
	} else if log.TenantID != "" && log.Cost > 0 {
		if _, _, err := r.deductWallet(tx,
			`(username = ? OR email = ?) AND role != 'admin'`, []any{log.TenantID, log.TenantID}, log.Cost); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// BatchRecordUsageLogs writes multiple usage logs in a single atomic transaction with aggregated cost updates.
func (r *Repository) BatchRecordUsageLogs(logs []*UsageLogRecord) error {
	return r.BatchRecordUsageLogsContext(context.Background(), logs)
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
	if len(logs) == 1 {
		return r.RecordUsageLogContext(ctx, logs[0])
	}

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	keyCostMap := make(map[string]float64)
	keyTokensMap := make(map[string]int64)
	keyTenantMap := make(map[string]string)
	orphanTenantCostMap := make(map[string]float64)

	// Multi-row INSERT: a 100-row flush used to be 100 round trips inside the
	// transaction. 23 columns per row keeps a chunk of 25 well under SQLite's
	// bound-parameter limit while cutting the round trips 25x.
	const (
		insertColumns  = `trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_units, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, error_message, pricing_snapshot, created_at, billing_id`
		insertColumnsN = 23
		insertChunk    = 25
	)

	// billingRow remembers the money-relevant fields of each queued row so the
	// aggregation below can be restricted to rows the database actually
	// inserted. A redelivered stream message and the local retry of the same
	// batch must not move a wallet twice.
	type billingRow struct {
		billingID string
		key       string
		tenant    string
		cost      float64
		tokens    int64
	}
	rowsMeta := make([]billingRow, 0, len(logs))
	seenIDs := make(map[string]struct{}, len(logs))

	var rowArgs []interface{}
	for _, log := range logs {
		if log == nil {
			continue
		}
		isOff := 0
		if log.IsOffPeak {
			isOff = 1
		}
		discount := log.OffPeakDiscount
		if discount <= 0 {
			discount = 1.0
		}
		if log.TraceID == "" {
			if log.SessionID != "" {
				log.TraceID = log.SessionID
			} else {
				log.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixMicro())
			}
		}

		keyVal := log.APIKey
		usageSource := normalizeUsageSource(log.UsageSource)
		// Every column is supplied: rows in one VALUES list must describe the
		// same shape, so a zero CreatedAt takes the flush timestamp instead of
		// the column default.
		createdAt := log.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		billingID := log.BillingID
		_, duplicateInBatch := seenIDs[billingID]
		if billingID == "" || duplicateInBatch {
			billingID = NewBillingID()
		}
		log.BillingID = billingID
		seenIDs[billingID] = struct{}{}

		rowArgs = append(rowArgs,
			log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel,
			log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToUnits(log.Cost),
			isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, usageSource,
			truncateError(log.ErrorMessage), encodePricingSnapshot(log.PricingSnapshot),
			createdAt.UTC().Format(sqliteTimestampLayout), billingID,
		)
		rowsMeta = append(rowsMeta, billingRow{
			billingID: billingID,
			key:       keyVal,
			tenant:    log.TenantID,
			cost:      log.Cost,
			tokens:    int64(log.TotalTokens),
		})
	}

	inserted := make(map[string]struct{}, len(rowsMeta))
	rowCount := len(rowArgs) / insertColumnsN
	if rowCount > 0 {
		rowPlaceholder := "(" + strings.Repeat("?, ", insertColumnsN-1) + "?)"
		for start := 0; start < rowCount; start += insertChunk {
			end := start + insertChunk
			if end > rowCount {
				end = rowCount
			}
			placeholders := make([]string, end-start)
			for i := range placeholders {
				placeholders[i] = rowPlaceholder
			}
			// RETURNING tells us which rows were new. ON CONFLICT turns a
			// redelivery into a no-op instead of a second charge.
			query := `INSERT INTO usage_logs (` + insertColumns + `) VALUES ` + strings.Join(placeholders, ",") + ` ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id`
			rows, err := tx.Query(query, rowArgs[start*insertColumnsN:end*insertColumnsN]...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				inserted[id] = struct{}{}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
		}
	}

	// Aggregate cost/quota only for rows that were actually inserted. Rows
	// already present (at-least-once redelivery) were charged by the delivery
	// that inserted them.
	skipped := 0
	for _, m := range rowsMeta {
		if _, ok := inserted[m.billingID]; !ok {
			skipped++
			continue
		}
		if m.key != "" {
			keyCostMap[m.key] += m.cost
			keyTokensMap[m.key] += m.tokens
			if m.tenant != "" {
				keyTenantMap[m.key] = m.tenant
			}
		} else if m.tenant != "" && m.cost > 0 {
			orphanTenantCostMap[m.tenant] += m.cost
		}
	}
	if skipped > 0 {
		telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(uint64(skipped))
	}

	// Aggregated batch update for API keys
	updateKeySQL := r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_units = COALESCE(used_cost_units, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`)
	updateKeyStmt, err := tx.Prepare(updateKeySQL)
	if err != nil {
		return err
	}
	defer updateKeyStmt.Close()
	for k, cost := range keyCostMap {
		tokens := keyTokensMap[k]
		if cost > 0 || tokens > 0 {
			if _, err := updateKeyStmt.Exec(cost, ToUnits(cost), tokens, k); err != nil {
				return err
			}
		}
	}

	// Aggregated batch update for user wallet balances. Each deduction is
	// clamped at the overdraft floor and reports back whether it applied, so the
	// admin-exemption and tenant-fallback rules stay exactly as before.
	{
		for k, cost := range keyCostMap {
			if cost <= 0 {
				continue
			}
			applied, _, err := r.deductWallet(tx,
				`id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`, []any{k}, cost)
			if err != nil {
				return err
			}
			if applied {
				continue
			}
			var isOwnerAdmin bool
			row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), k)
			if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
				// Admin keys are exempt; do not fallback to tenant
				continue
			}
			// Delegate to tenant deduction
			if tID := keyTenantMap[k]; tID != "" {
				orphanTenantCostMap[tID] += cost
			}
		}
	}

	// Deduct for orphan or keyless tenant requests exactly once
	if len(orphanTenantCostMap) > 0 {
		for tenantID, tCost := range orphanTenantCostMap {
			if tCost <= 0 {
				continue
			}
			if _, _, err := r.deductWallet(tx,
				`(username = ? OR email = ?) AND role != 'admin'`, []any{tenantID, tenantID}, tCost); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
