package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ErrChannelNotFound reports that a channel operation targeted a row that does
// not exist. It is deliberately distinct from a read failure so a caller can
// answer 404 for a missing channel and 500 for a broken database.
var ErrChannelNotFound = errors.New("channel not found")

// ChannelRecord represents the database row for channels.
type ChannelRecord struct {
	ID                int64                                  `json:"id"`
	Name              string                                 `json:"name"`
	Type              model.ProviderType                     `json:"type"`
	BaseURL           string                                 `json:"base_url"`
	APIKey            string                                 `json:"api_key"`
	ModelCapabilities map[string]model.ModelCapabilityConfig `json:"model_capabilities,omitempty"`
	Models            []string                               `json:"models"`
	ModelMapping      map[string]string                      `json:"model_mapping"`
	Protocols         []string                               `json:"protocols,omitempty"`
	Priority          int                                    `json:"priority"`
	Weight            int                                    `json:"weight"`
	TimeoutSeconds    int                                    `json:"timeout_seconds"`
	MaxConcurrency    int                                    `json:"max_concurrency"`
	QueueLimit        int                                    `json:"queue_limit"`
	QueueWaitSeconds  int                                    `json:"queue_wait_seconds"`
	Status            string                                 `json:"status"` // active, inactive
	BreakerStatus     string                                 `json:"breaker_status,omitempty"`
	CreatedAt         time.Time                              `json:"created_at"`
	UpdatedAt         time.Time                              `json:"updated_at"`
}

// CreateChannel inserts a new channel.
func (r *Repository) CreateChannel(rec *ChannelRecord) error {
	modelsBytes, _ := json.Marshal(rec.Models)
	mappingBytes, _ := json.Marshal(rec.ModelMapping)
	protocolsBytes, _ := json.Marshal(rec.Protocols)
	capabilitiesBytes, _ := json.Marshal(rec.ModelCapabilities)
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

	id, err := r.db.InsertGetID(`INSERT INTO channels (name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, max_concurrency, queue_limit, queue_wait_seconds, status, model_capabilities, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.MaxConcurrency, rec.QueueLimit, rec.QueueWaitSeconds, rec.Status, string(capabilitiesBytes))
	if err != nil {
		return err
	}
	rec.ID = id
	// The database stamped the timestamps; echo them so the creation response does
	// not report a zero time to the console.
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = now
	}
	return nil
}

// PatchChannel applies fn to the stored channel and writes the result back, all
// within one transaction.
//
// It exists because reading a channel, merging a change in Go and then writing
// the whole row back loses concurrent edits: an administrator who saves a
// credential change between another administrator's read and write has that
// change silently reverted. PatchChannel re-reads the row at write time, so a
// field the caller did not touch keeps whatever value it holds then.
//
// Returning an error from fn aborts the patch and leaves the stored row
// untouched.
func (r *Repository) PatchChannel(id int64, fn func(*ChannelRecord) error) (*ChannelRecord, error) {
	if fn == nil {
		return nil, fmt.Errorf("PatchChannel requires a mutation function")
	}

	tx, err := r.db.BeginTx(context.Background())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	rec, err := getChannelTx(tx, id)
	if err != nil {
		return nil, err
	}

	if err := fn(rec); err != nil {
		return nil, err
	}

	if err := updateChannelTx(tx, rec); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return rec, nil
}

// UpdateChannel updates an existing channel.
//
// Unlike CreateChannel, it stores the routing fields exactly as given: priority 0,
// weight 0 and timeout_seconds 0 are valid values on an update (the create path
// still maps them to 1/10/60 because there an omitted field means "use the
// default"). Both update entry points share updateChannelTx so they cannot drift.
func (r *Repository) UpdateChannel(rec *ChannelRecord) error {
	if rec == nil {
		return fmt.Errorf("UpdateChannel requires a channel record")
	}
	tx, err := r.db.BeginTx(context.Background())
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if err := updateChannelTx(tx, rec); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteChannel deletes a channel by ID.
func (r *Repository) DeleteChannel(id int64) error {
	_, err := r.db.Exec(`DELETE FROM channels WHERE id=?`, id)
	if err == nil {
		r.pruneChannelsFromModelRoutes(id)
	}
	return err
}

// BatchDeleteChannels deletes multiple channels by their IDs.
func (r *Repository) BatchDeleteChannels(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders, args := inClause[int64](nil, ids)
	q := fmt.Sprintf(`DELETE FROM channels WHERE id IN (%s)`, placeholders)
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	r.pruneChannelsFromModelRoutes(ids...)
	return res.RowsAffected()
}

func (r *Repository) pruneChannelsFromModelRoutes(ids ...int64) {
	if r == nil || r.db == nil || len(ids) == 0 {
		return
	}
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	rows, err := r.db.Query(`SELECT model, providers FROM model_routes WHERE providers IS NOT NULL AND providers != ''`)
	if err != nil {
		return
	}
	defer rows.Close()

	type routeUpdate struct {
		model     string
		providers any
	}
	var updates []routeUpdate

	for rows.Next() {
		var m, provJSON string
		if err := rows.Scan(&m, &provJSON); err != nil {
			continue
		}
		var provs []model.RouteProvider
		if err := json.Unmarshal([]byte(provJSON), &provs); err != nil {
			continue
		}
		var kept []model.RouteProvider
		changed := false
		for _, p := range provs {
			if idSet[p.ChannelID] {
				changed = true
			} else {
				kept = append(kept, p)
			}
		}
		if changed {
			var newProv any
			if len(kept) > 0 {
				if b, err := json.Marshal(kept); err == nil {
					newProv = string(b)
				}
			}
			updates = append(updates, routeUpdate{model: m, providers: newProv})
		}
	}

	for _, u := range updates {
		_, _ = r.db.Exec(`UPDATE model_routes SET providers = ?, updated_at = CURRENT_TIMESTAMP WHERE model = ?`, u.providers, u.model)
	}
}

// BatchUpdateChannelStatus updates status ('active'/'disabled') for multiple channels.
func (r *Repository) BatchUpdateChannelStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders, args := inClause[int64]([]any{status}, ids)
	q := fmt.Sprintf(`UPDATE channels SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, placeholders)
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
