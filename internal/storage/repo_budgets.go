package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/budget"
)

// BudgetRecord is the stored form of one budget rule.
type BudgetRecord struct {
	ID          int64      `json:"id"`
	ScopeType   string     `json:"scope_type"`
	ScopeID     int64      `json:"scope_id"`
	Period      string     `json:"period"`
	AmountUnits int64      `json:"amount_units"`
	UsedUnits   int64      `json:"used_units"`
	Action      string     `json:"action"`
	WarnPercent int        `json:"warn_percent"`
	Status      string     `json:"status"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ToRule converts the row into the pure evaluation type.
func (r *BudgetRecord) ToRule() budget.Rule {
	return budget.Rule{
		ID:          r.ID,
		ScopeType:   budget.ScopeType(r.ScopeType),
		ScopeID:     r.ScopeID,
		Period:      budget.Period(r.Period),
		AmountUnits: r.AmountUnits,
		UsedUnits:   r.UsedUnits,
		Action:      budget.Action(r.Action),
		WarnPercent: r.WarnPercent,
	}
}

func normalizeBudgetRecord(rec *BudgetRecord) error {
	if rec == nil {
		return fmt.Errorf("budget is required")
	}
	rec.ScopeType = strings.ToLower(strings.TrimSpace(rec.ScopeType))
	rec.Period = strings.ToLower(strings.TrimSpace(rec.Period))
	rec.Action = strings.ToLower(strings.TrimSpace(rec.Action))
	rec.Status = strings.ToLower(strings.TrimSpace(rec.Status))
	if rec.Action == "" {
		rec.Action = string(budget.ActionNotify)
	}
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.WarnPercent == 0 {
		rec.WarnPercent = 80
	}
	if rec.Status != "active" && rec.Status != "disabled" {
		return fmt.Errorf("invalid budget status %q", rec.Status)
	}
	return rec.ToRule().Validate()
}

// UpsertBudget creates or updates a rule. used_units is intentionally preserved
// on update: editing the amount must not erase measured spending.
func (r *Repository) UpsertBudget(rec *BudgetRecord) error {
	if err := normalizeBudgetRecord(rec); err != nil {
		return err
	}
	_, err := r.db.Exec(`INSERT INTO budgets
		(scope_type, scope_id, period, amount_units, used_units, action, warn_percent, status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (scope_type, scope_id, period) DO UPDATE SET
			amount_units = excluded.amount_units,
			action = excluded.action,
			warn_percent = excluded.warn_percent,
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP`,
		rec.ScopeType, rec.ScopeID, rec.Period, rec.AmountUnits, rec.UsedUnits, rec.Action, rec.WarnPercent, rec.Status)
	if err != nil {
		return err
	}
	stored, err := r.GetBudgetByScope(rec.ScopeType, rec.ScopeID, rec.Period)
	if err != nil {
		return err
	}
	*rec = *stored
	return nil
}

func scanBudget(row interface{ Scan(...any) error }) (*BudgetRecord, error) {
	var rec BudgetRecord
	var periodStart sql.NullTime
	if err := row.Scan(&rec.ID, &rec.ScopeType, &rec.ScopeID, &rec.Period, &rec.AmountUnits,
		&rec.UsedUnits, &rec.Action, &rec.WarnPercent, &rec.Status, &periodStart,
		&rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if periodStart.Valid {
		value := periodStart.Time
		rec.PeriodStart = &value
	}
	return &rec, nil
}

const budgetColumns = `id, scope_type, scope_id, period, amount_units, used_units, action, warn_percent, status, period_start, created_at, updated_at`

// GetBudget loads one rule by id.
func (r *Repository) GetBudget(id int64) (*BudgetRecord, error) {
	return scanBudget(r.db.QueryRow(`SELECT `+budgetColumns+` FROM budgets WHERE id = ?`, id))
}

// GetBudgetByScope loads one rule by its business key.
func (r *Repository) GetBudgetByScope(scopeType string, scopeID int64, period string) (*BudgetRecord, error) {
	return scanBudget(r.db.QueryRow(`SELECT `+budgetColumns+` FROM budgets
		WHERE scope_type = ? AND scope_id = ? AND period = ?`, scopeType, scopeID, period))
}

// ListBudgets returns budgets for one scope, or all budgets when scopeID is 0.
func (r *Repository) ListBudgets(scopeType string, scopeID int64) ([]*BudgetRecord, error) {
	query := `SELECT ` + budgetColumns + ` FROM budgets`
	args := make([]any, 0, 2)
	if scopeType != "" || scopeID > 0 {
		query += ` WHERE 1=1`
		if scopeType != "" {
			query += ` AND scope_type = ?`
			args = append(args, scopeType)
		}
		if scopeID > 0 {
			query += ` AND scope_id = ?`
			args = append(args, scopeID)
		}
	}
	query += ` ORDER BY scope_type ASC, scope_id ASC, period ASC`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*BudgetRecord, 0)
	for rows.Next() {
		rec, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, rows.Err()
}

// DeleteBudget removes a rule; its historical events remain.
func (r *Repository) DeleteBudget(id int64) error {
	result, err := r.db.Exec(`DELETE FROM budgets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetBudgetStatus enables or disables a rule.
func (r *Repository) SetBudgetStatus(id int64, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "active" && status != "disabled" {
		return fmt.Errorf("invalid budget status %q", status)
	}
	result, err := r.db.Exec(`UPDATE budgets SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// IncrementBudgetUsage adds measured spend to a rule. A negative delta is
// ignored: budget counters only move forward within a period.
func (r *Repository) IncrementBudgetUsage(id, deltaUnits int64) error {
	if deltaUnits <= 0 {
		return nil
	}
	_, err := r.db.Exec(`UPDATE budgets SET used_units = used_units + ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, deltaUnits, id)
	return err
}

// RecordBudgetEvent inserts a threshold event once per (budget, threshold,
// period). It returns whether this call was the one that inserted it, so the
// notifier does not send duplicates on every request above the threshold.
func (r *Repository) RecordBudgetEvent(budgetID int64, threshold int, periodKey string, usedUnits, amountUnits int64, action string) (bool, error) {
	result, err := r.db.Exec(`INSERT INTO budget_events (budget_id, threshold, period_key, used_units, amount_units, action)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (budget_id, threshold, period_key) DO NOTHING`,
		budgetID, threshold, periodKey, usedUnits, amountUnits, action)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ListBudgetEvents returns the newest events for a rule.
func (r *Repository) ListBudgetEvents(budgetID int64, limit int) ([]*BudgetEventRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(`SELECT id, budget_id, threshold, period_key, used_units, amount_units, action, created_at
		FROM budget_events WHERE budget_id = ? ORDER BY id DESC LIMIT ?`, budgetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*BudgetEventRecord, 0)
	for rows.Next() {
		var event BudgetEventRecord
		if err := rows.Scan(&event.ID, &event.BudgetID, &event.Threshold, &event.PeriodKey,
			&event.UsedUnits, &event.AmountUnits, &event.Action, &event.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &event)
	}
	return list, rows.Err()
}

// BudgetEventRecord is one emitted threshold crossing.
type BudgetEventRecord struct {
	ID          int64     `json:"id"`
	BudgetID    int64     `json:"budget_id"`
	Threshold   int       `json:"threshold"`
	PeriodKey   string    `json:"period_key"`
	UsedUnits   int64     `json:"used_units"`
	AmountUnits int64     `json:"amount_units"`
	Action      string    `json:"action"`
	CreatedAt   time.Time `json:"created_at"`
}

// EffectiveBudgetRules resolves the active budgets that apply to one request.
// The caller passes the key, its owner and the organization chain ordered from
// the key's own organization to the root; budget.Resolve applies the
// key -> user -> nearest-organization precedence per period.
func (r *Repository) EffectiveBudgetRules(keyID, userID int64, orgChain []int64) ([]budget.Rule, error) {
	conditions := []string{
		"(scope_type = ? AND scope_id = ?)",
		"(scope_type = ? AND scope_id = ?)",
	}
	args := []any{string(budget.ScopeKey), keyID, string(budget.ScopeUser), userID}
	if len(orgChain) > 0 {
		placeholders := make([]string, len(orgChain))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		conditions = append(conditions, "(scope_type = ? AND scope_id IN ("+strings.Join(placeholders, ",")+"))")
		args = append(args, string(budget.ScopeOrg))
		for _, orgID := range orgChain {
			args = append(args, orgID)
		}
	}
	query := `SELECT ` + budgetColumns + ` FROM budgets WHERE status = 'active' AND (` + strings.Join(conditions, " OR ") + `)`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]budget.Rule, 0)
	for rows.Next() {
		rec, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, rec.ToRule())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return budget.Resolve(candidates, keyID, userID, orgChain), nil
}
