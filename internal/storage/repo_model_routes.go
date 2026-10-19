package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// ModelRouteBundle is everything a single "save model route" action must apply:
// the cross-model fallback rule, the pricing row for one group, and the set of
// channel rows whose model list / mapping / weight changed.
//
// It exists so the whole action commits or rolls back as one unit. Previously
// the handler wrote these three concerns through three independent
// transactions: if pricing or channel persistence failed, the fallback (or the
// price) had already been committed and the operator saw an error while the
// configuration was left half-applied.
type ModelRouteBundle struct {
	Model string
	// FallbackModel empty means "remove the fallback rule for Model".
	FallbackModel string
	// Price nil means "pricing was not part of this request".
	Price *ModelPriceRecord
	// Channels are full replacement rows (same semantics as UpdateChannel).
	Channels []*ChannelRecord
}

// ApplyModelRouteBundle writes the whole route change in one transaction.
func (r *Repository) ApplyModelRouteBundle(ctx context.Context, b *ModelRouteBundle) error {
	if b == nil || strings.TrimSpace(b.Model) == "" {
		return nil
	}

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Fallback rule.
	if b.FallbackModel != "" {
		if _, err := tx.Exec(`
			INSERT INTO model_fallbacks (model, fallback_model, enabled, updated_at)
			VALUES (?, ?, 1, CURRENT_TIMESTAMP)
			ON CONFLICT(model) DO UPDATE SET fallback_model = excluded.fallback_model, enabled = 1, updated_at = CURRENT_TIMESTAMP
		`, b.Model, b.FallbackModel); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`DELETE FROM model_fallbacks WHERE model = ?`, b.Model); err != nil {
			return err
		}
	}

	// 2. Pricing row for the requested group.
	if b.Price != nil {
		if err := upsertModelPriceTx(tx, b.Price); err != nil {
			return err
		}
	}

	// 3. Channel rows.
	for _, ch := range b.Channels {
		if ch == nil {
			continue
		}
		if err := updateChannelTx(tx, ch); err != nil {
			return err
		}
	}

	return tx.Commit()
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
