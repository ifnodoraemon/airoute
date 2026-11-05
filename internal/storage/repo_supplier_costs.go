package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SupplierCostRecord is the contracted cost of one model on one channel. Rates
// are stored as 1e-9 units of Currency per million tokens or per request.
// A nil optional rate is unknown; an explicit zero is free.
type SupplierCostRecord struct {
	Currency                      string    `json:"currency"`
	BillingMode                   string    `json:"billing_mode"`
	CacheReadCostUnitsPerMillion  *int64    `json:"cache_read_cost_units_per_million,omitempty"`
	CacheWriteCostUnitsPerMillion *int64    `json:"cache_write_cost_units_per_million,omitempty"`
	FixedCostUnits                *int64    `json:"fixed_cost_units,omitempty"`
	ID                            int64     `json:"id"`
	Channel                       string    `json:"channel"`
	Model                         string    `json:"model"`
	InputCostUnitsPerMillion      int64     `json:"input_cost_units_per_million"`
	OutputCostUnitsPerMillion     int64     `json:"output_cost_units_per_million"`
	Status                        string    `json:"status"`
	CreatedAt                     time.Time `json:"created_at"`
	UpdatedAt                     time.Time `json:"updated_at"`
}

func normalizeSupplierCost(rec *SupplierCostRecord) error {
	if rec == nil {
		return fmt.Errorf("supplier cost is required")
	}
	rec.Channel = strings.TrimSpace(rec.Channel)
	rec.Model = strings.TrimSpace(rec.Model)
	rec.Status = strings.ToLower(strings.TrimSpace(rec.Status))
	rec.Currency = strings.ToUpper(strings.TrimSpace(rec.Currency))
	if rec.Currency == "" {
		rec.Currency = "CNY"
	}
	if len(rec.Currency) != 3 {
		return fmt.Errorf("currency must be a three-letter code")
	}
	for _, letter := range rec.Currency {
		if letter < 'A' || letter > 'Z' {
			return fmt.Errorf("currency must be a three-letter code")
		}
	}
	rec.BillingMode = strings.ToLower(strings.TrimSpace(rec.BillingMode))
	if rec.BillingMode == "" {
		rec.BillingMode = "token"
	}
	if rec.BillingMode != "token" && rec.BillingMode != "request" {
		return fmt.Errorf("billing mode must be token or request")
	}
	for _, value := range []*int64{rec.CacheReadCostUnitsPerMillion, rec.CacheWriteCostUnitsPerMillion, rec.FixedCostUnits} {
		if value != nil && *value < 0 {
			return fmt.Errorf("supplier prices must not be negative")
		}
	}
	if rec.BillingMode == "request" {
		if rec.FixedCostUnits == nil {
			return fmt.Errorf("per-request purchase price is required")
		}
		rec.InputCostUnitsPerMillion, rec.OutputCostUnitsPerMillion = 0, 0
		rec.CacheReadCostUnitsPerMillion, rec.CacheWriteCostUnitsPerMillion = nil, nil
	} else {
		rec.FixedCostUnits = nil
	}
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

const supplierCostColumns = `id, channel, model, input_cost_units_per_million, output_cost_units_per_million, status, created_at, updated_at, currency, billing_mode, cache_read_cost_units_per_million, cache_write_cost_units_per_million, fixed_cost_units`

func scanSupplierCost(row interface{ Scan(...any) error }) (*SupplierCostRecord, error) {
	var rec SupplierCostRecord
	if err := row.Scan(&rec.ID, &rec.Channel, &rec.Model, &rec.InputCostUnitsPerMillion,
		&rec.OutputCostUnitsPerMillion, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &rec.Currency, &rec.BillingMode, &rec.CacheReadCostUnitsPerMillion, &rec.CacheWriteCostUnitsPerMillion, &rec.FixedCostUnits); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpsertSupplierCost creates or updates one (channel, model) price.
func (r *Repository) UpsertSupplierCost(rec *SupplierCostRecord) error {
	if err := normalizeSupplierCost(rec); err != nil {
		return err
	}
	err := upsertSupplierCost(r.db, rec)
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

// BatchUpsertSupplierCosts leaves no partial import behind on a write failure.
func (r *Repository) BatchUpsertSupplierCosts(ctx context.Context, records []*SupplierCostRecord) error {
	for _, rec := range records {
		if err := normalizeSupplierCost(rec); err != nil {
			return err
		}
	}
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, rec := range records {
		if err := upsertSupplierCost(tx, rec); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type supplierCostWriter interface {
	Exec(string, ...any) (sql.Result, error)
}

func upsertSupplierCost(db supplierCostWriter, rec *SupplierCostRecord) error {
	_, err := db.Exec(`INSERT INTO supplier_costs
		(channel, model, input_cost_units_per_million, output_cost_units_per_million, status, currency, billing_mode, cache_read_cost_units_per_million, cache_write_cost_units_per_million, fixed_cost_units, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (channel, model) DO UPDATE SET
			input_cost_units_per_million = excluded.input_cost_units_per_million,
			output_cost_units_per_million = excluded.output_cost_units_per_million,
			status = excluded.status,
			currency = excluded.currency,
			billing_mode = excluded.billing_mode,
			cache_read_cost_units_per_million = excluded.cache_read_cost_units_per_million,
			cache_write_cost_units_per_million = excluded.cache_write_cost_units_per_million,
			fixed_cost_units = excluded.fixed_cost_units,
			updated_at = CURRENT_TIMESTAMP`,
		rec.Channel, rec.Model, rec.InputCostUnitsPerMillion, rec.OutputCostUnitsPerMillion, rec.Status, rec.Currency, rec.BillingMode, rec.CacheReadCostUnitsPerMillion, rec.CacheWriteCostUnitsPerMillion, rec.FixedCostUnits)
	return err
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

// ValidateSupplierCost exposes the same validation the repository applies, so
// bulk import can report every bad row before writing anything.
func ValidateSupplierCost(rec *SupplierCostRecord) error {
	return normalizeSupplierCost(rec)
}
