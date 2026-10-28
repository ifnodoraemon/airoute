package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ModelPriceRecord defines rates for a model.
type ModelPriceRecord struct {
	ID              int64     `json:"id"`
	Model           string    `json:"model"`
	GroupName       string    `json:"group_name"`        // "default", "vip", "enterprise", etc.
	PromptPrice     float64   `json:"prompt_price"`      // per 1,000,000 prompt tokens (CNY/USD)
	CompletionPrice float64   `json:"completion_price"`  // per 1,000,000 completion tokens
	CacheReadPrice  float64   `json:"cache_read_price"`  // per 1,000,000 cached tokens
	FixedPrice      float64   `json:"fixed_price"`       // per request (e.g. image/video)
	Currency        string    `json:"currency"`          // CNY; wallets settle exclusively in CNY
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

// BatchDeleteModelPrices deletes pricing for multiple models.
func (r *Repository) BatchDeleteModelPrices(models []string) (int64, error) {
	if len(models) == 0 {
		return 0, nil
	}
	placeholders, args := inClause[string](nil, models)
	q := fmt.Sprintf(`DELETE FROM model_prices WHERE model IN (%s)`, placeholders)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

// SetModelFallbacks creates or updates a set of model fallback mappings in one
// transaction, so a caller that rewrites the fallback table cannot leave half of
// it applied when a statement fails.
//
// Every entry is written enabled: the fallback rows the gateway serves are the
// enabled ones (GetModelFallbacks filters on enabled = 1), and disabling a
// mapping is expressed by removing it.
func (r *Repository) SetModelFallbacks(fallbacks map[string]string) error {
	if len(fallbacks) == 0 {
		return nil
	}
	models := make([]string, 0, len(fallbacks))
	for model := range fallbacks {
		models = append(models, model)
	}
	sort.Strings(models)

	tx, err := r.db.BeginTx(context.Background())
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	for _, model := range models {
		if _, err := tx.Exec(`
			INSERT INTO model_fallbacks (model, fallback_model, enabled, updated_at)
			VALUES (?, ?, 1, CURRENT_TIMESTAMP)
			ON CONFLICT(model) DO UPDATE SET fallback_model = excluded.fallback_model, enabled = 1, updated_at = CURRENT_TIMESTAMP
		`, model, fallbacks[model]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListModelPrices returns all configured model pricing rates.
//
// It reads through Reader(): the billing engine reloads this list on cache misses,
// and on SQLite it used to queue behind the single settlement writer (one slow
// flush delayed every price lookup on the request path).
func (r *Repository) ListModelPrices() ([]*ModelPriceRecord, error) {
	rows, err := r.db.Reader().Query(r.db.Rebind(`SELECT id, model, COALESCE(group_name, 'default'), prompt_price, completion_price, cache_read_price, fixed_price, currency, COALESCE(off_peak_enabled, 1), COALESCE(off_peak_start, '00:00'), COALESCE(off_peak_end, '08:30'), COALESCE(off_peak_discount, 0.5), COALESCE(off_peak_mode, 'custom'), COALESCE(off_peak_slots, ''), COALESCE(weekend_all_day, 1), created_at, updated_at FROM model_prices ORDER BY id ASC`))
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
	// Only "not found" falls back to the default group; a database failure must
	// surface instead of silently pricing the request with the default rate.
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if groupName != "default" {
		return r.GetModelPriceExact(modelName, "default")
	}
	return nil, sql.ErrNoRows
}

// normalizeModelPrice applies the defaults every price row must have before it
// is persisted. Shared by SaveModelPrice and the transactional route bundle.
func normalizeModelPrice(rec *ModelPriceRecord) {
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	if rec.Currency == "" {
		rec.Currency = "CNY"
	}
	if rec.OffPeakMode == "" {
		rec.OffPeakMode = "custom"
	}
	// Never synthesise an off-peak slot list here.
	//
	// This used to write [{"start":"00:00","end":"08:30","discount":0.5}] whenever
	// a row enabled off-peak without explicit slots. billing.CustomOffPeakStrategy
	// evaluates the legacy single window (off_peak_start/off_peak_end) only when
	// the slot list is EMPTY, so the synthetic slot silently replaced the
	// operator's window: a configured 22:00-06:00 @0.25 window charged full price
	// at 23:00 and 0.5 (instead of 0.25) at 03:00. An empty slot list is a
	// legitimate configuration meaning "use the single window".
	if rec.OffPeakStart == "" {
		rec.OffPeakStart = "00:00"
	}
	if rec.OffPeakEnd == "" {
		rec.OffPeakEnd = "08:30"
	}
	if rec.OffPeakDiscount <= 0 || rec.OffPeakDiscount > 1.0 {
		// 0 (or an unset field) means "no discount". It used to become 0.5, so a
		// rule the operator created without a coefficient silently billed half
		// price. An out-of-range factor is clamped to full price for the same
		// reason: never invent a discount.
		rec.OffPeakDiscount = 1.0
	}
}

// SaveModelPrice inserts or updates pricing rates for a model within a specific group.
func (r *Repository) SaveModelPrice(rec *ModelPriceRecord) error {
	if err := ValidatePriceCurrency(rec); err != nil {
		return err
	}
	normalizeModelPrice(rec)

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

// ValidatePriceCurrency rejects currencies the CNY ledger cannot settle.
func ValidatePriceCurrency(rec *ModelPriceRecord) error {
	if rec != nil && rec.Currency != "" && rec.Currency != "CNY" {
		return errors.New("当前钱包仅支持 CNY 定价；请明确换算后以 CNY 保存")
	}
	return nil
}
