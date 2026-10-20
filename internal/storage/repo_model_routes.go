package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ChannelRoutingUpdate describes a change to one channel's routing, expressed as a
// delta rather than a replacement row.
//
// This distinction is load-bearing. A replacement row is built from a read taken
// before the transaction, so two administrators who each add a different model to
// the same shared channel would overwrite one another: the second save writes the
// model list it read, and the first administrator's model disappears. A delta is
// applied to the row as it exists inside the transaction, so concurrent additions
// to different models both survive.
type ChannelRoutingUpdate struct {
	ID int64
	// AddModels are model names the channel must serve. Adding one it already
	// serves is a no-op, which is what makes concurrent adds commutative.
	AddModels []string
	// RemoveModels are model names the channel must stop serving.
	RemoveModels []string
	// SetMapping are model_mapping entries to write.
	SetMapping map[string]string
	// RemoveMapping are model_mapping keys to delete.
	RemoveMapping []string
	// Priority and Weight are applied only when positive: zero means "leave it",
	// matching the form, where an unset field must not reset a stored value.
	Priority int
	Weight   int
}

// ModelRouteBundle is everything a single "save model route" action must apply:
// the cross-model fallback rule, the pricing row for one group, and the routing
// changes for the channels that serve the model.
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
	// Channels carries routing deltas. Each is applied to the channel as it exists
	// inside the transaction, so a concurrent edit to any field — including the
	// model list — survives. Naming a channel that no longer exists aborts the
	// whole bundle.
	Channels []ChannelRoutingUpdate
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
	//
	// Each row is re-read inside the transaction and only the fields the
	// model-route editor owns are taken from the caller's copy. Writing the whole
	// row from the caller's earlier read would also reset name, base_url, api_key,
	// protocols, timeout and status to whatever they held at read time, silently
	// undoing a concurrent edit to a field the route change has no opinion about —
	// the channel modal and the route editor are different screens, edited by
	// different people.
	for _, u := range b.Channels {
		current, err := getChannelTx(tx, u.ID)
		if err != nil {
			if errors.Is(err, ErrChannelNotFound) {
				// The plan was built from a read taken before this transaction, so
				// the channel can have been deleted in between. Failing rolls the
				// whole change back rather than silently applying a partial plan:
				// the operator asked for a route change that can no longer be
				// applied as specified.
				return fmt.Errorf("planned channel %d no longer exists: %w", u.ID, err)
			}
			return err
		}

		// Apply the delta to the row as it exists now, then write only if that
		// actually changed something. Writing unconditionally would turn every
		// route save into a write of every channel row.
		if !applyChannelRoutingUpdate(current, u) {
			continue
		}

		if err := updateChannelTx(tx, current); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// applyChannelRoutingUpdate folds a routing delta into rec and reports whether
// anything changed.
func applyChannelRoutingUpdate(rec *ChannelRecord, u ChannelRoutingUpdate) bool {
	changed := false

	if len(u.AddModels) > 0 {
		present := make(map[string]bool, len(rec.Models))
		for _, m := range rec.Models {
			present[m] = true
		}
		for _, m := range u.AddModels {
			if m == "" || present[m] {
				continue
			}
			rec.Models = append(rec.Models, m)
			present[m] = true
			changed = true
		}
	}

	if len(u.RemoveModels) > 0 {
		drop := make(map[string]bool, len(u.RemoveModels))
		for _, m := range u.RemoveModels {
			drop[m] = true
		}
		kept := make([]string, 0, len(rec.Models))
		for _, m := range rec.Models {
			if drop[m] {
				changed = true
				continue
			}
			kept = append(kept, m)
		}
		rec.Models = kept
	}

	if len(u.SetMapping) > 0 {
		if rec.ModelMapping == nil {
			rec.ModelMapping = make(map[string]string, len(u.SetMapping))
		}
		for k, v := range u.SetMapping {
			if rec.ModelMapping[k] == v {
				continue
			}
			rec.ModelMapping[k] = v
			changed = true
		}
	}

	if len(u.RemoveMapping) > 0 {
		for _, k := range u.RemoveMapping {
			if _, exists := rec.ModelMapping[k]; !exists {
				continue
			}
			delete(rec.ModelMapping, k)
			changed = true
		}
	}

	if u.Priority > 0 && rec.Priority != u.Priority {
		rec.Priority = u.Priority
		changed = true
	}
	if u.Weight > 0 && rec.Weight != u.Weight {
		rec.Weight = u.Weight
		changed = true
	}

	return changed
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
