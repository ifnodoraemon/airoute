package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ModelPriceRecord defines rates for a model.
type ModelPriceRecord struct {
	ID              int64     `json:"id"`
	Model           string    `json:"model"`
	GroupName       string    `json:"group_name"`       // "default", "vip", "enterprise", etc.
	PromptPrice     float64   `json:"prompt_price"`     // per 1,000,000 prompt tokens (CNY/USD)
	CompletionPrice float64   `json:"completion_price"` // per 1,000,000 completion tokens
	CacheReadPrice  float64   `json:"cache_read_price"` // per 1,000,000 cached tokens
	FixedPrice      float64   `json:"fixed_price"`      // per request (e.g. image/video)
	Currency        string    `json:"currency"`          // CNY or USD
	OffPeakEnabled  bool      `json:"off_peak_enabled"`  // whether time-of-use discount is active
	OffPeakMode     string    `json:"off_peak_mode"`     // "custom", "night", "none"
	OffPeakSlots    string    `json:"off_peak_slots"`    // JSON array of OffPeakSlot
	WeekendAllDay   bool      `json:"weekend_all_day"`   // whether weekends are all-day off-peak
	OffPeakStart    string    `json:"off_peak_start"`    // fallback/simple start
	OffPeakEnd      string    `json:"off_peak_end"`      // fallback/simple end
	OffPeakDiscount float64   `json:"off_peak_discount"` // e.g. 0.5 (50% discount)
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ModelPriceKey identifies a specific model within a pricing group.
type ModelPriceKey struct {
	Model string `json:"model"`
	Group string `json:"group"`
}

// ModelFallbackRecord represents a cross-model fallback rule.
type ModelFallbackRecord struct {
	Model         string    `json:"model"`
	FallbackModel string    `json:"fallback_model"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BatchDeleteModelPrices deletes pricing for multiple models.
func (r *Repository) BatchDeleteModelPrices(models []string) (int64, error) {
	if len(models) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(models))
	args := make([]interface{}, len(models))
	for i, m := range models {
		placeholders[i] = "?"
		args[i] = m
	}
	q := fmt.Sprintf(`DELETE FROM model_prices WHERE model IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchDeleteModelPriceKeys deletes specific (model, group) combinations.
func (r *Repository) BatchDeleteModelPriceKeys(keys []ModelPriceKey) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	var total int64
	for _, k := range keys {
		grp := strings.TrimSpace(k.Group)
		if grp == "" {
			grp = "default"
		}
		res, err := r.db.Exec(`DELETE FROM model_prices WHERE model = ? AND group_name = ?`, k.Model, grp)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// GetModelFallbacks returns all active model fallbacks mapped as target -> fallback.
func (r *Repository) GetModelFallbacks() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT model, fallback_model FROM model_fallbacks WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err == nil && src != "" && dst != "" {
			m[src] = dst
		}
	}
	return m, nil
}

// SetModelFallback creates or updates a model fallback mapping.
func (r *Repository) SetModelFallback(model, fallbackModel string, enabled bool) error {
	en := 1
	if !enabled {
		en = 0
	}
	_, err := r.db.Exec(`
		INSERT INTO model_fallbacks (model, fallback_model, enabled, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(model) DO UPDATE SET fallback_model = excluded.fallback_model, enabled = excluded.enabled, updated_at = CURRENT_TIMESTAMP
	`, model, fallbackModel, en)
	return err
}

// DeleteModelFallback removes a model fallback mapping.
func (r *Repository) DeleteModelFallback(model string) error {
	_, err := r.db.Exec(`DELETE FROM model_fallbacks WHERE model = ?`, model)
	return err
}

// ListModelPrices returns all configured model pricing rates.
func (r *Repository) ListModelPrices() ([]*ModelPriceRecord, error) {
	rows, err := r.db.Query(`SELECT id, model, COALESCE(group_name, 'default'), prompt_price, completion_price, cache_read_price, fixed_price, currency, COALESCE(off_peak_enabled, 1), COALESCE(off_peak_start, '00:00'), COALESCE(off_peak_end, '08:30'), COALESCE(off_peak_discount, 0.5), COALESCE(off_peak_mode, 'custom'), COALESCE(off_peak_slots, ''), COALESCE(weekend_all_day, 1), created_at, updated_at FROM model_prices ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*ModelPriceRecord, 0)
	for rows.Next() {
		var rec ModelPriceRecord
		var offEnabled, weekendAllDay int
		if err := rows.Scan(&rec.ID, &rec.Model, &rec.GroupName, &rec.PromptPrice, &rec.CompletionPrice, &rec.CacheReadPrice, &rec.FixedPrice, &rec.Currency, &offEnabled, &rec.OffPeakStart, &rec.OffPeakEnd, &rec.OffPeakDiscount, &rec.OffPeakMode, &rec.OffPeakSlots, &weekendAllDay, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		rec.OffPeakEnabled = offEnabled == 1
		rec.WeekendAllDay = weekendAllDay == 1
		list = append(list, &rec)
	}
	return list, nil
}

// GetModelPrice returns pricing rates for a specific model (defaulting to default group).
func (r *Repository) GetModelPrice(modelName string) (*ModelPriceRecord, error) {
	return r.GetModelPriceWithGroup(modelName, "default")
}

// GetModelPriceExact returns pricing rates for an exact model and group without fallback.
func (r *Repository) GetModelPriceExact(modelName, groupName string) (*ModelPriceRecord, error) {
	if groupName == "" {
		groupName = "default"
	}
	row := r.db.QueryRow(`SELECT id, model, COALESCE(group_name, 'default'), prompt_price, completion_price, cache_read_price, fixed_price, currency, COALESCE(off_peak_enabled, 1), COALESCE(off_peak_start, '00:00'), COALESCE(off_peak_end, '08:30'), COALESCE(off_peak_discount, 0.5), COALESCE(off_peak_mode, 'custom'), COALESCE(off_peak_slots, ''), COALESCE(weekend_all_day, 1), created_at, updated_at FROM model_prices WHERE model = ? AND group_name = ?`, modelName, groupName)
	var rec ModelPriceRecord
	var offEnabled, weekendAllDay int
	if err := row.Scan(&rec.ID, &rec.Model, &rec.GroupName, &rec.PromptPrice, &rec.CompletionPrice, &rec.CacheReadPrice, &rec.FixedPrice, &rec.Currency, &offEnabled, &rec.OffPeakStart, &rec.OffPeakEnd, &rec.OffPeakDiscount, &rec.OffPeakMode, &rec.OffPeakSlots, &weekendAllDay, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	rec.OffPeakEnabled = offEnabled == 1
	rec.WeekendAllDay = weekendAllDay == 1
	return &rec, nil
}

// GetModelPriceWithGroup returns pricing rates for a specific model and group (with fallback to default group).
func (r *Repository) GetModelPriceWithGroup(modelName, groupName string) (*ModelPriceRecord, error) {
	if groupName == "" {
		groupName = "default"
	}
	rec, err := r.GetModelPriceExact(modelName, groupName)
	if err == nil {
		return rec, nil
	}
	// Fallback to default group if specific group not found
	if groupName != "default" {
		return r.GetModelPriceExact(modelName, "default")
	}
	return nil, sql.ErrNoRows
}

// SaveModelPrice inserts or updates pricing rates for a model within a specific group.
func (r *Repository) SaveModelPrice(rec *ModelPriceRecord) error {
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	if rec.Currency == "" {
		rec.Currency = "CNY"
	}
	if rec.OffPeakMode == "" {
		rec.OffPeakMode = "custom"
	}
	if rec.OffPeakSlots == "" && rec.OffPeakEnabled {
		rec.OffPeakSlots = `[{"start":"00:00","end":"08:30","discount":0.5}]`
	}
	if rec.OffPeakStart == "" {
		rec.OffPeakStart = "00:00"
	}
	if rec.OffPeakEnd == "" {
		rec.OffPeakEnd = "08:30"
	}
	if rec.OffPeakDiscount <= 0 || rec.OffPeakDiscount > 1.0 {
		rec.OffPeakDiscount = 0.5
	}
	offEnabled := 0
	if rec.OffPeakEnabled {
		offEnabled = 1
	}
	weekendAll := 0
	if rec.WeekendAllDay {
		weekendAll = 1
	}

	_, err := r.db.Exec(`
		INSERT INTO model_prices (model, group_name, prompt_price, completion_price, cache_read_price, fixed_price, currency, off_peak_enabled, off_peak_start, off_peak_end, off_peak_discount, off_peak_mode, off_peak_slots, weekend_all_day, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(model, group_name) DO UPDATE SET 
			prompt_price = excluded.prompt_price,
			completion_price = excluded.completion_price,
			cache_read_price = excluded.cache_read_price,
			fixed_price = excluded.fixed_price,
			currency = excluded.currency,
			off_peak_enabled = excluded.off_peak_enabled,
			off_peak_start = excluded.off_peak_start,
			off_peak_end = excluded.off_peak_end,
			off_peak_discount = excluded.off_peak_discount,
			off_peak_mode = excluded.off_peak_mode,
			off_peak_slots = excluded.off_peak_slots,
			weekend_all_day = excluded.weekend_all_day,
			updated_at = CURRENT_TIMESTAMP
	`, rec.Model, rec.GroupName, rec.PromptPrice, rec.CompletionPrice, rec.CacheReadPrice, rec.FixedPrice, rec.Currency, offEnabled, rec.OffPeakStart, rec.OffPeakEnd, rec.OffPeakDiscount, rec.OffPeakMode, rec.OffPeakSlots, weekendAll)
	return err
}

// DeleteModelPrice removes pricing rates for a model.
func (r *Repository) DeleteModelPrice(modelName string) error {
	_, err := r.db.Exec(`DELETE FROM model_prices WHERE model = ?`, modelName)
	return err
}

// DeleteModelPriceWithGroup removes pricing rates for a specific model and group.
func (r *Repository) DeleteModelPriceWithGroup(modelName, groupName string) error {
	if groupName == "" {
		groupName = "default"
	}
	_, err := r.db.Exec(`DELETE FROM model_prices WHERE model = ? AND group_name = ?`, modelName, groupName)
	return err
}


