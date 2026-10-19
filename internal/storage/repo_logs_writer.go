package storage

import (
	"fmt"
	"time"
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

// normalizeUsageSource clamps the usage provenance marker to a known value.
func normalizeUsageSource(src string) string {
	if src == "estimated" {
		return "estimated"
	}
	return "upstream"
}

// RecordUsageLog records an audit log and updates key quota/cost atomically.
func (r *Repository) RecordUsageLog(log *UsageLogRecord) error {
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
			log.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixNano())
		}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	keyVal := log.APIKey
	usageSource := normalizeUsageSource(log.UsageSource)

	if !log.CreatedAt.IsZero() {
		_, err = tx.Exec(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_nano, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToNano(log.Cost), isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, usageSource, log.CreatedAt.UTC().Format("2006-01-02 15:04:05"))
	} else {
		_, err = tx.Exec(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_nano, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToNano(log.Cost), isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, usageSource)
	}
	if err != nil {
		return err
	}

	if keyVal != "" && (log.Cost > 0 || log.TotalTokens > 0) {
		if _, err := tx.Exec(r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_nano = COALESCE(used_cost_nano, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`),
			log.Cost, ToNano(log.Cost), log.TotalTokens, keyVal); err != nil {
			return err
		}
		if log.Cost > 0 {
			res, err := tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance - ?, balance_nano = COALESCE(balance_nano, 0) - ?, updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`),
				log.Cost, ToNano(log.Cost), keyVal)
			if err != nil {
				return err
			}
			if res != nil {
				affected, _ := res.RowsAffected()
				if affected == 0 && log.TenantID != "" {
					var isOwnerAdmin bool
					row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), keyVal)
					if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
						// Admin keys are exempt from balance deduction; do not fallback to tenant
					} else {
						if _, err := tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance - ?, balance_nano = COALESCE(balance_nano, 0) - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`),
							log.Cost, ToNano(log.Cost), log.TenantID, log.TenantID); err != nil {
							return err
						}
					}
				}
			}
		}
	} else if log.TenantID != "" && log.Cost > 0 {
		if _, err := tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance - ?, balance_nano = COALESCE(balance_nano, 0) - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`),
			log.Cost, ToNano(log.Cost), log.TenantID, log.TenantID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// BatchRecordUsageLogs writes multiple usage logs in a single atomic transaction with aggregated cost updates.
func (r *Repository) BatchRecordUsageLogs(logs []*UsageLogRecord) error {
	if len(logs) == 0 {
		return nil
	}
	if len(logs) == 1 {
		return r.RecordUsageLog(logs[0])
	}
	for _, log := range logs {
		normalizeUsageLog(log)
	}

	tx, err := r.db.Begin()
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

	insertWithTimeSQL := r.db.Rebind(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_nano, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	insertSQL := r.db.Rebind(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_nano, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	stmtWithTime, err := tx.Prepare(insertWithTimeSQL)
	if err != nil {
		return err
	}
	defer stmtWithTime.Close()

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

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
				log.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixNano())
			}
		}

		keyVal := log.APIKey
		usageSource := normalizeUsageSource(log.UsageSource)
		if !log.CreatedAt.IsZero() {
			_, err = stmtWithTime.Exec(log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToNano(log.Cost), isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, usageSource, log.CreatedAt.UTC().Format("2006-01-02 15:04:05"))
		} else {
			_, err = stmt.Exec(log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToNano(log.Cost), isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, usageSource)
		}
		if err != nil {
			return err
		}

		if keyVal != "" {
			keyCostMap[keyVal] += log.Cost
			keyTokensMap[keyVal] += int64(log.TotalTokens)
			if log.TenantID != "" {
				keyTenantMap[keyVal] = log.TenantID
			}
		} else if log.TenantID != "" && log.Cost > 0 {
			orphanTenantCostMap[log.TenantID] += log.Cost
		}
	}

	// Aggregated batch update for API keys
	updateKeySQL := r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_nano = COALESCE(used_cost_nano, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`)
	updateKeyStmt, err := tx.Prepare(updateKeySQL)
	if err != nil {
		return err
	}
	defer updateKeyStmt.Close()
	for k, cost := range keyCostMap {
		tokens := keyTokensMap[k]
		if cost > 0 || tokens > 0 {
			if _, err := updateKeyStmt.Exec(cost, ToNano(cost), tokens, k); err != nil {
				return err
			}
		}
	}

	// Aggregated batch update for user wallet balances
	updateUserSQL := r.db.Rebind(`UPDATE users SET balance = balance - ?, balance_nano = COALESCE(balance_nano, 0) - ?, updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`)
	updateUserStmt, err := tx.Prepare(updateUserSQL)
	if err != nil {
		return err
	}
	defer updateUserStmt.Close()
	{
		for k, cost := range keyCostMap {
			if cost > 0 {
				res, _ := updateUserStmt.Exec(cost, ToNano(cost), k)
				if res != nil {
					affected, _ := res.RowsAffected()
					if affected == 0 {
						var isOwnerAdmin bool
						row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), k)
						if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
							// Admin keys are exempt; do not fallback to tenant
						} else {
							// Delegate to tenant deduction
							if tID := keyTenantMap[k]; tID != "" {
								orphanTenantCostMap[tID] += cost
							}
						}
					}
				}
			}
		}
	}

	// Deduct for orphan or keyless tenant requests exactly once
	if len(orphanTenantCostMap) > 0 {
		fallbackSQL := r.db.Rebind(`UPDATE users SET balance = balance - ?, balance_nano = COALESCE(balance_nano, 0) - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`)
		for tenantID, tCost := range orphanTenantCostMap {
			if tCost > 0 {
				if _, err := tx.Exec(fallbackSQL, tCost, ToNano(tCost), tenantID, tenantID); err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit()
}
