package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ChannelRecord represents the database row for channels.
type ChannelRecord struct {
	ID             int64              `json:"id"`
	Name           string             `json:"name"`
	Type           model.ProviderType `json:"type"`
	BaseURL        string             `json:"base_url"`
	APIKey         string             `json:"api_key"`
	Models         []string           `json:"models"`
	ModelMapping   map[string]string  `json:"model_mapping"`
	Protocols      []string           `json:"protocols,omitempty"`
	Priority       int                `json:"priority"`
	Weight         int                `json:"weight"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	Status         string             `json:"status"` // active, inactive
	BreakerStatus  string             `json:"breaker_status,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// ListChannels returns all channels.
func (r *Repository) ListChannels() ([]*ChannelRecord, error) {
	rows, err := r.db.Query(`SELECT id, name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, created_at, updated_at FROM channels ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ChannelRecord
	for rows.Next() {
		var rec ChannelRecord
		var modelsJSON, mappingJSON, protocolsJSON sql.NullString
		err := rows.Scan(&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.APIKey, &modelsJSON, &mappingJSON, &protocolsJSON, &rec.Priority, &rec.Weight, &rec.TimeoutSeconds, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if modelsJSON.Valid && modelsJSON.String != "" {
			_ = json.Unmarshal([]byte(modelsJSON.String), &rec.Models)
		}
		if mappingJSON.Valid && mappingJSON.String != "" {
			_ = json.Unmarshal([]byte(mappingJSON.String), &rec.ModelMapping)
		}
		if protocolsJSON.Valid && protocolsJSON.String != "" {
			_ = json.Unmarshal([]byte(protocolsJSON.String), &rec.Protocols)
		}
		list = append(list, &rec)
	}
	return list, nil
}

// CreateChannel inserts a new channel.
func (r *Repository) CreateChannel(rec *ChannelRecord) error {
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

	id, err := r.db.InsertGetID(`INSERT INTO channels (name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.Status)
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}

// GetChannel retrieves a single channel by ID.
func (r *Repository) GetChannel(id int64) (*ChannelRecord, error) {
	row := r.db.QueryRow(`SELECT id, name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, created_at, updated_at FROM channels WHERE id = ?`, id)
	var rec ChannelRecord
	var modelsJSON, mappingJSON, protocolsJSON sql.NullString
	if err := row.Scan(&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.APIKey, &modelsJSON, &mappingJSON, &protocolsJSON, &rec.Priority, &rec.Weight, &rec.TimeoutSeconds, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if modelsJSON.Valid {
		_ = json.Unmarshal([]byte(modelsJSON.String), &rec.Models)
	}
	if mappingJSON.Valid {
		_ = json.Unmarshal([]byte(mappingJSON.String), &rec.ModelMapping)
	}
	if protocolsJSON.Valid {
		_ = json.Unmarshal([]byte(protocolsJSON.String), &rec.Protocols)
	}
	return &rec, nil
}

// UpdateChannel updates an existing channel.
func (r *Repository) UpdateChannel(rec *ChannelRecord) error {
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

	_, err := r.db.Exec(`UPDATE channels SET name=?, type=?, base_url=?, api_key=?, models=?, model_mapping=?, protocols=?, priority=?, weight=?, timeout_seconds=?, status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.Status, rec.ID)
	return err
}

// DeleteChannel deletes a channel by ID.
func (r *Repository) DeleteChannel(id int64) error {
	_, err := r.db.Exec(`DELETE FROM channels WHERE id=?`, id)
	return err
}

// BatchDeleteChannels deletes multiple channels by their IDs.
func (r *Repository) BatchDeleteChannels(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM channels WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateChannelStatus updates status ('active'/'disabled') for multiple channels.
func (r *Repository) BatchUpdateChannelStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+1] = id
	}
	q := fmt.Sprintf(`UPDATE channels SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ToModelChannels converts database ChannelRecords to Data Plane model.ChannelConfigs.
func (r *Repository) ToModelChannels() ([]model.ChannelConfig, error) {
	records, err := r.ListChannels()
	if err != nil {
		return nil, err
	}
	var res []model.ChannelConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		res = append(res, model.ChannelConfig{
			ID:             rec.ID,
			Name:           rec.Name,
			Type:           rec.Type,
			BaseURL:        rec.BaseURL,
			APIKey:         rec.APIKey,
			Models:         rec.Models,
			ModelMapping:   rec.ModelMapping,
			Protocols:      rec.Protocols,
			Priority:       rec.Priority,
			Weight:         rec.Weight,
			TimeoutSeconds: rec.TimeoutSeconds,
			Status:         rec.Status,
		})
	}
	return res, nil
}
