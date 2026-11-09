package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// channelColumns is the projection shared by every single-channel read.
const channelColumns = `id, name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, max_concurrency, queue_limit, queue_wait_seconds, status, created_at, updated_at, model_capabilities`

// scanChannel reads one channel row, decoding its JSON columns.
func scanChannel(row rowScanner) (*ChannelRecord, error) {
	var rec ChannelRecord
	var modelsJSON, mappingJSON, protocolsJSON, capabilitiesJSON sql.NullString
	if err := row.Scan(&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.APIKey,
		&modelsJSON, &mappingJSON, &protocolsJSON, &rec.Priority, &rec.Weight,
		&rec.TimeoutSeconds, &rec.MaxConcurrency, &rec.QueueLimit, &rec.QueueWaitSeconds, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &capabilitiesJSON); err != nil {
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
	if capabilitiesJSON.Valid && capabilitiesJSON.String != "" {
		if err := json.Unmarshal([]byte(capabilitiesJSON.String), &rec.ModelCapabilities); err != nil {
			return nil, fmt.Errorf("decode channel capabilities: %w", err)
		}
	}
	return &rec, nil
}

// ListChannels returns all channels.
func (r *Repository) ListChannels() ([]*ChannelRecord, error) {
	rows, err := r.db.Query(`SELECT ` + channelColumns + ` FROM channels ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ChannelRecord
	for rows.Next() {
		rec, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// getChannelTx reads one channel inside a transaction. On PostgreSQL the row is
// locked for the remainder of the transaction, which is what makes PatchChannel
// safe against a concurrent writer. SQLite has no row locks and rejects FOR
// UPDATE; it serialises writers through its single pooled connection instead.
func getChannelTx(tx *Tx, id int64) (*ChannelRecord, error) {
	query := `SELECT ` + channelColumns + ` FROM channels WHERE id = ?`
	if tx.db.Dialect() == "postgres" {
		query += " FOR UPDATE"
	}

	rec, err := scanChannel(tx.QueryRow(query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Both sentinels are wrapped so callers can match either one.
			return nil, fmt.Errorf("%w: channel %d does not exist: %w", ErrChannelNotFound, id, err)
		}
		return nil, err
	}
	return rec, nil
}

// ToModelChannels converts database ChannelRecords to Data Plane model.ChannelConfigs.
func (r *Repository) ToModelChannels() ([]model.ChannelConfig, error) {
	records, err := r.ListChannels()
	if err != nil {
		return nil, err
	}
	rules, err := r.ListModelRouteRules()
	if err != nil {
		return nil, err
	}
	var res []model.ChannelConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		res = append(res, model.ChannelConfig{
			RouteRules:        rules,
			ID:                rec.ID,
			Name:              rec.Name,
			Type:              rec.Type,
			BaseURL:           rec.BaseURL,
			APIKey:            rec.APIKey,
			Models:            rec.Models,
			ModelMapping:      rec.ModelMapping,
			ModelCapabilities: rec.ModelCapabilities,
			Protocols:         rec.Protocols,
			Priority:          rec.Priority,
			Weight:            rec.Weight,
			TimeoutSeconds:    rec.TimeoutSeconds,
			MaxConcurrency:    rec.MaxConcurrency,
			QueueLimit:        rec.QueueLimit,
			QueueWaitSeconds:  rec.QueueWaitSeconds,
			Status:            rec.Status,
		})
	}
	return res, nil
}
