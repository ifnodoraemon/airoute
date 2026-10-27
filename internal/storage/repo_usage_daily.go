package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/budget"
)

// UsageDailyRecord is one pre-aggregated day/org/channel/model bucket.
type UsageDailyRecord struct {
	Day               string `json:"day"`
	OrgID             int64  `json:"org_id"`
	ChannelID         int64  `json:"channel_id"`
	Model             string `json:"model"`
	Requests          int64  `json:"requests"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	BilledUnits       int64  `json:"billed_units"`
	CostBasisUnits    int64  `json:"cost_basis_units"`
	KnownCostRequests int64  `json:"known_cost_requests"`
}

const usageDailyColumns = `day, org_id, channel_id, model, requests, prompt_tokens, completion_tokens, total_tokens, billed_units, cost_basis_units, known_cost_requests`

// ChannelIDByName resolves the stable channel id used by the audit row and the
// daily rollup.
func (r *Repository) ChannelIDByName(name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("channel name is required")
	}
	var id int64
	if err := r.db.QueryRow(`SELECT id FROM channels WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// RefreshUsageDaily rebuilds one UTC day from usage_logs. It is idempotent:
// delete-then-insert inside a transaction, so a retried job cannot double-count.
func (r *Repository) RefreshUsageDaily(ctx context.Context, day string) error {
	day = strings.TrimSpace(day)
	start, err := time.Parse("2006-01-02", day)
	if err != nil {
		return fmt.Errorf("invalid day %q: %w", day, err)
	}
	end := start.Add(24 * time.Hour)
	startText := start.Format(sqliteTimestampLayout)
	// The exclusive end must not carry microseconds: a row stamped exactly at
	// 00:00:00 would otherwise sort before "00:00:00.000000" and leak into the
	// previous day.
	endText := end.Format("2006-01-02 15:04:05")

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_daily WHERE day = ?`, day); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_daily
		(day, org_id, channel_id, model, requests, prompt_tokens, completion_tokens, total_tokens, billed_units, cost_basis_units, known_cost_requests)
		SELECT ?, COALESCE(org_id, 0), COALESCE(channel_id, 0), COALESCE(model, ''), COUNT(*),
			COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(cost_units), 0), COALESCE(SUM(cost_basis_units), 0), COUNT(cost_basis_units)
		FROM usage_logs WHERE created_at >= ? AND created_at < ?
		GROUP BY COALESCE(org_id, 0), COALESCE(channel_id, 0), COALESCE(model, '')`,
		day, startText, endText)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// EnsureUsageDaily refreshes every day in [start, end]. It is bounded to 92 days
// so an admin request cannot trigger an unbounded rebuild.
func (r *Repository) EnsureUsageDaily(ctx context.Context, start, end string) error {
	startDay, err := time.Parse("2006-01-02", strings.TrimSpace(start))
	if err != nil {
		return fmt.Errorf("invalid start day: %w", err)
	}
	endDay, err := time.Parse("2006-01-02", strings.TrimSpace(end))
	if err != nil {
		return fmt.Errorf("invalid end day: %w", err)
	}
	if endDay.Before(startDay) {
		return fmt.Errorf("end day is before start day")
	}
	if endDay.Sub(startDay) > 92*24*time.Hour {
		return fmt.Errorf("range exceeds 92 days")
	}
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		if err := r.RefreshUsageDaily(ctx, day.Format("2006-01-02")); err != nil {
			return err
		}
	}
	return nil
}

// ListUsageDaily returns rollup rows for a range.
func (r *Repository) ListUsageDaily(start, end string) ([]*UsageDailyRecord, error) {
	rows, err := r.db.Query(`SELECT `+usageDailyColumns+` FROM usage_daily
		WHERE day >= ? AND day <= ? ORDER BY day ASC, org_id ASC, channel_id ASC, model ASC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*UsageDailyRecord, 0)
	for rows.Next() {
		var rec UsageDailyRecord
		if err := rows.Scan(&rec.Day, &rec.OrgID, &rec.ChannelID, &rec.Model, &rec.Requests,
			&rec.PromptTokens, &rec.CompletionTokens, &rec.TotalTokens, &rec.BilledUnits,
			&rec.CostBasisUnits, &rec.KnownCostRequests); err != nil {
			return nil, err
		}
		list = append(list, &rec)
	}
	return list, rows.Err()
}

// CostTrend aggregates usage_daily by day.
func (r *Repository) CostTrend(start, end string) ([]CostGroup, error) {
	rows, err := r.db.Query(`SELECT day, SUM(requests), COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(billed_units), 0), COALESCE(SUM(cost_basis_units), 0), COALESCE(SUM(known_cost_requests), 0)
		FROM usage_daily WHERE day >= ? AND day <= ? GROUP BY day ORDER BY day ASC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]CostGroup, 0)
	for rows.Next() {
		var group CostGroup
		if err := rows.Scan(&group.Key, &group.Requests, &group.TotalTokens, &group.BilledUnits,
			&group.CostBasisUnits, &group.KnownCostRequests); err != nil {
			return nil, err
		}
		group.Label = group.Key
		list = append(list, group)
	}
	return list, rows.Err()
}

// ReconcileBudgets recomputes active budgets from usage_logs. The live counters
// are authoritative within a period; this daily job corrects drift after
// restarts, missed flushes or manual data repair.
func (r *Repository) ReconcileBudgets(now time.Time) (int, error) {
	records, err := r.ListBudgets("", 0)
	if err != nil {
		return 0, err
	}
	reconciled := 0
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		period := budget.Period(rec.Period)
		if !period.Valid() {
			continue
		}
		var used int64
		var queryErr error
		switch budget.ScopeType(rec.ScopeType) {
		case budget.ScopeOrg:
			queryErr = r.sumCostUnits(&used, `org_id = ?`, rec.ScopeID, period, now)
		case budget.ScopeUser:
			queryErr = r.sumCostUnits(&used, `api_key IN (SELECT key FROM api_keys WHERE user_id = ?)`, rec.ScopeID, period, now)
		case budget.ScopeKey:
			queryErr = r.sumCostUnits(&used, `api_key IN (SELECT key FROM api_keys WHERE id = ?)`, rec.ScopeID, period, now)
		default:
			continue
		}
		if queryErr != nil {
			return reconciled, queryErr
		}
		if _, err := r.db.Exec(`UPDATE budgets SET used_units = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, used, rec.ID); err != nil {
			return reconciled, err
		}
		reconciled++
	}
	return reconciled, nil
}

func (r *Repository) sumCostUnits(dest *int64, where string, arg int64, period budget.Period, now time.Time) error {
	query := `SELECT COALESCE(SUM(cost_units), 0) FROM usage_logs WHERE ` + where
	args := []any{arg}
	if start := period.Start(now); !start.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, start.UTC().Format(sqliteTimestampLayout))
	}
	return r.db.QueryRow(query, args...).Scan(dest)
}
