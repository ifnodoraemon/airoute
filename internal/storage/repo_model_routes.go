package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

type ModelRouteProvider = model.RouteProvider

// ModelRouteBundle commits route policy, fallback and optional pricing together.
// Providers nil leaves the existing policy intact (or inherits channel defaults
// for a new route); an empty, non-nil list explicitly selects no providers.
type ModelRouteBundle struct {
	Model         string
	FallbackModel string
	Price         *ModelPriceRecord
	Providers     []ModelRouteProvider
}

func createModelRoutesTable(db *DB) error {
	_, err := db.DB.Exec(`CREATE TABLE IF NOT EXISTS model_routes (
  model TEXT PRIMARY KEY,
  disabled INTEGER NOT NULL DEFAULT 0,
  providers TEXT,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
 )`)
	return err
}

// ApplyModelRouteBundle never writes channel capabilities or channel defaults.
func (r *Repository) ApplyModelRouteBundle(ctx context.Context, b *ModelRouteBundle) error {
	if b == nil || strings.TrimSpace(b.Model) == "" {
		return nil
	}
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if b.FallbackModel != "" {
		_, err = tx.Exec(`INSERT INTO model_fallbacks (model, fallback_model, enabled, updated_at)
   VALUES (?, ?, 1, CURRENT_TIMESTAMP)
   ON CONFLICT(model) DO UPDATE SET fallback_model=excluded.fallback_model, enabled=1, updated_at=CURRENT_TIMESTAMP`, b.Model, b.FallbackModel)
	} else {
		_, err = tx.Exec(`DELETE FROM model_fallbacks WHERE model=?`, b.Model)
	}
	if err != nil {
		return err
	}
	if b.Price != nil {
		if err := upsertModelPriceTx(tx, b.Price); err != nil {
			return err
		}
	}
	var providers any
	if b.Providers != nil {
		// Validate within the transaction: a channel may disappear after planning.
		for _, p := range b.Providers {
			if _, err := getChannelTx(tx, p.ChannelID); err != nil {
				return fmt.Errorf("route channel %d is unavailable: %w", p.ChannelID, err)
			}
		}
		raw, err := json.Marshal(b.Providers)
		if err != nil {
			return err
		}
		providers = string(raw)
	}
	_, err = tx.Exec(`INSERT INTO model_routes (model, disabled, providers) VALUES (?, 0, ?)
  ON CONFLICT(model) DO UPDATE SET disabled=0,
   providers=COALESCE(excluded.providers, model_routes.providers), updated_at=CURRENT_TIMESTAMP`, b.Model, providers)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteModelRoutes suppresses automatic routes durably without changing the
// channel catalog. The tombstone is cleared by an explicit route save.
func (r *Repository) DeleteModelRoutes(ctx context.Context, models []string) error {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, name := range models {
		if _, err := tx.Exec(`INSERT INTO model_routes (model, disabled, providers) VALUES (?, 1, NULL)
   ON CONFLICT(model) DO UPDATE SET disabled=1, providers=NULL, updated_at=CURRENT_TIMESTAMP`, name); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM model_fallbacks WHERE model=?`, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListModelRouteRules() (map[string]model.ModelRouteRule, error) {
	rows, err := r.db.Query(`SELECT model, disabled, providers FROM model_routes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make(map[string]model.ModelRouteRule)
	for rows.Next() {
		var name string
		var disabled int
		var raw sql.NullString
		if err := rows.Scan(&name, &disabled, &raw); err != nil {
			return nil, err
		}
		rule := model.ModelRouteRule{Disabled: disabled != 0}
		if raw.Valid {
			if err := json.Unmarshal([]byte(raw.String), &rule.Providers); err != nil {
				return nil, fmt.Errorf("decode route %q: %w", name, err)
			}
		}
		rules[name] = rule
	}
	return rules, rows.Err()
}

// upsertModelPriceTx mirrors SaveModelPrice but participates in a transaction.
func upsertModelPriceTx(tx *Tx, rec *ModelPriceRecord) error {
	normalizeModelPrice(rec)

	offEnabled := 0
	if rec.OffPeakEnabled {
		offEnabled = 1
	}
	weekendAll := 0
	if rec.WeekendAllDay {
		weekendAll = 1
	}

	_, err := tx.Exec(`
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

// updateChannelTx mirrors UpdateChannel but participates in a transaction.
func updateChannelTx(tx *Tx, rec *ChannelRecord) error {
	modelsBytes, _ := json.Marshal(rec.Models)
	mappingBytes, _ := json.Marshal(rec.ModelMapping)
	protocolsBytes, _ := json.Marshal(rec.Protocols)

	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.Priority == 0 {
		rec.Priority = 1
	}
	if rec.Weight == 0 {
		rec.Weight = 10
	}
	if rec.TimeoutSeconds == 0 {
		rec.TimeoutSeconds = 60
	}

	_, err := tx.Exec(`UPDATE channels SET name=?, type=?, base_url=?, api_key=?, models=?, model_mapping=?, protocols=?, priority=?, weight=?, timeout_seconds=?, status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.Status, rec.ID)
	return err
}

// CountChannelsServingModel returns how many active channels serve the model.
func (r *Repository) CountChannelsServingModel(modelName string) (int, error) {
	rows, err := r.db.Query(`SELECT models FROM channels WHERE status = 'active' OR status IS NULL OR status = ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil {
			return count, err
		}
		if !raw.Valid || raw.String == "" {
			continue
		}
		var models []string
		if err := json.Unmarshal([]byte(raw.String), &models); err != nil {
			continue
		}
		for _, m := range models {
			if m == modelName {
				count++
				break
			}
		}
	}
	return count, rows.Err()
}
