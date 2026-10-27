package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SupplierCostRecord is the contracted cost of one model on one channel. Rates
// are stored as 1e-9 CNY per one million tokens so arithmetic stays integer.
type SupplierCostRecord struct {
	ID                        int64     `json:"id"`
	Channel                   string    `json:"channel"`
	Model                     string    `json:"model"`
	InputCostUnitsPerMillion  int64     `json:"input_cost_units_per_million"`
	OutputCostUnitsPerMillion int64     `json:"output_cost_units_per_million"`
	Status                    string    `json:"status"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

func normalizeSupplierCost(rec *SupplierCostRecord) error {
	if rec == nil {
		return fmt.Errorf("supplier cost is required")
	}
	rec.Channel = strings.TrimSpace(rec.Channel)
	rec.Model = strings.TrimSpace(rec.Model)
	rec.Status = strings.ToLower(strings.TrimSpace(rec.Status))
	if rec.Channel == "" {
		return fmt.Errorf("supplier cost channel is required")
	}
	if rec.Model == "" {
		return fmt.Errorf("supplier cost model is required")
	}
	if rec.InputCostUnitsPerMillion < 0 || rec.OutputCostUnitsPerMillion < 0 {
		return fmt.Errorf("supplier cost rates must not be negative")
	}
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.Status != "active" && rec.Status != "disabled" {
		return fmt.Errorf("invalid supplier cost status %q", rec.Status)
	}
	return nil
}

const supplierCostColumns = `id, channel, model, input_cost_units_per_million, output_cost_units_per_million, status, created_at, updated_at`

func scanSupplierCost(row interface{ Scan(...any) error }) (*SupplierCostRecord, error) {
	var rec SupplierCostRecord
	if err := row.Scan(&rec.ID, &rec.Channel, &rec.Model, &rec.InputCostUnitsPerMillion,
		&rec.OutputCostUnitsPerMillion, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpsertSupplierCost creates or updates one (channel, model) price.
func (r *Repository) UpsertSupplierCost(rec *SupplierCostRecord) error {
	if err := normalizeSupplierCost(rec); err != nil {
		return err
	}
	_, err := r.db.Exec(`INSERT INTO supplier_costs
		(channel, model, input_cost_units_per_million, output_cost_units_per_million, status, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (channel, model) DO UPDATE SET
			input_cost_units_per_million = excluded.input_cost_units_per_million,
			output_cost_units_per_million = excluded.output_cost_units_per_million,
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP`,
		rec.Channel, rec.Model, rec.InputCostUnitsPerMillion, rec.OutputCostUnitsPerMillion, rec.Status)
	if err != nil {
		return err
	}
	stored, err := r.GetSupplierCost(rec.Channel, rec.Model)
	if err != nil {
		return err
	}
	*rec = *stored
	return nil
}

// GetSupplierCost loads the price for one channel+model.
func (r *Repository) GetSupplierCost(channel, model string) (*SupplierCostRecord, error) {
	return scanSupplierCost(r.db.QueryRow(`SELECT `+supplierCostColumns+` FROM supplier_costs
		WHERE channel = ? AND model = ?`, strings.TrimSpace(channel), strings.TrimSpace(model)))
}

// ListSupplierCosts returns active and disabled prices for the console.
func (r *Repository) ListSupplierCosts() ([]*SupplierCostRecord, error) {
	rows, err := r.db.Query(`SELECT ` + supplierCostColumns + ` FROM supplier_costs ORDER BY channel ASC, model ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*SupplierCostRecord, 0)
	for rows.Next() {
		rec, err := scanSupplierCost(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, rows.Err()
}

// DeleteSupplierCost removes a price.
func (r *Repository) DeleteSupplierCost(id int64) error {
	result, err := r.db.Exec(`DELETE FROM supplier_costs WHERE id = ?`, id)
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

// SupplierCostUnits computes the cost basis of a request. found=false means no
// active price is configured; the caller stores NULL rather than zero.
func (r *Repository) SupplierCostUnits(channel, model string, promptTokens, completionTokens int64) (int64, bool, error) {
	rec, err := r.GetSupplierCost(channel, model)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}
	if rec.Status != "active" {
		return 0, false, nil
	}
	units := (promptTokens*rec.InputCostUnitsPerMillion + completionTokens*rec.OutputCostUnitsPerMillion) / 1_000_000
	return units, true, nil
}

// CostSummary aggregates one time range. CostBasis covers only rows with a
// known price; UnknownCostRequests makes that gap explicit instead of hiding it.
type CostSummary struct {
	Requests            int64 `json:"requests"`
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	BilledUnits         int64 `json:"billed_units"`
	CostBasisUnits      int64 `json:"cost_basis_units"`
	KnownCostRequests   int64 `json:"known_cost_requests"`
	UnknownCostRequests int64 `json:"unknown_cost_requests"`
}

func costRange(start, end string) (string, []any) {
	where := ` WHERE 1=1`
	args := make([]any, 0, 2)
	if strings.TrimSpace(start) != "" {
		where += ` AND created_at >= ?`
		args = append(args, start)
	}
	if strings.TrimSpace(end) != "" {
		where += ` AND created_at <= ?`
		args = append(args, end)
	}
	return where, args
}

// CostSummary aggregates billed revenue and supplier cost for a range.
func (r *Repository) CostSummary(start, end string) (*CostSummary, error) {
	where, args := costRange(start, end)
	var summary CostSummary
	err := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost_units),0), COALESCE(SUM(cost_basis_units),0),
		COUNT(cost_basis_units), COUNT(*) - COUNT(cost_basis_units)
		FROM usage_logs`+where, args...).Scan(&summary.Requests, &summary.PromptTokens, &summary.CompletionTokens,
		&summary.TotalTokens, &summary.BilledUnits, &summary.CostBasisUnits,
		&summary.KnownCostRequests, &summary.UnknownCostRequests)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// CostGroup is one row of a grouped cost report.
type CostGroup struct {
	Key               string `json:"key"`
	Label             string `json:"label,omitempty"`
	Requests          int64  `json:"requests"`
	TotalTokens       int64  `json:"total_tokens"`
	BilledUnits       int64  `json:"billed_units"`
	CostBasisUnits    int64  `json:"cost_basis_units"`
	KnownCostRequests int64  `json:"known_cost_requests"`
}

func (r *Repository) costGroups(groupExpr, join, labelExpr, start, end string) ([]CostGroup, error) {
	where, args := costRange(start, end)
	query := `SELECT ` + groupExpr + ` AS k, ` + labelExpr + ` AS label, COUNT(*),
		COALESCE(SUM(ul.total_tokens),0), COALESCE(SUM(ul.cost_units),0),
		COALESCE(SUM(ul.cost_basis_units),0), COUNT(ul.cost_basis_units)
		FROM usage_logs ul ` + join + where + ` GROUP BY k, label ORDER BY 5 DESC LIMIT 100`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]CostGroup, 0)
	for rows.Next() {
		var group CostGroup
		if err := rows.Scan(&group.Key, &group.Label, &group.Requests, &group.TotalTokens,
			&group.BilledUnits, &group.CostBasisUnits, &group.KnownCostRequests); err != nil {
			return nil, err
		}
		list = append(list, group)
	}
	return list, rows.Err()
}

// CostByChannel groups by the channel name recorded on the audit row.
func (r *Repository) CostByChannel(start, end string) ([]CostGroup, error) {
	return r.costGroups("ul.channel", "", "ul.channel", start, end)
}

// CostByModel groups by model.
func (r *Repository) CostByModel(start, end string) ([]CostGroup, error) {
	return r.costGroups("ul.model", "", "ul.model", start, end)
}

// CostByOrg groups by organization, joining the display name.
func (r *Repository) CostByOrg(start, end string) ([]CostGroup, error) {
	return r.costGroups("CAST(ul.org_id AS TEXT)", "LEFT JOIN orgs o ON o.id = ul.org_id", "COALESCE(o.name, '')", start, end)
}

// ValidateSupplierCost exposes the same validation the repository applies, so
// bulk import can report every bad row before writing anything.
func ValidateSupplierCost(rec *SupplierCostRecord) error {
	return normalizeSupplierCost(rec)
}
