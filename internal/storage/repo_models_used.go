package storage

import (
	"strings"
	"time"
)

// DistinctRecentModels lists the models that have actually served traffic since
// the given cutoff. It backs the startup self-check that warns about models
// billed at the hardcoded fallback rate.
func (r *Repository) DistinctRecentModels(since time.Time, limit int) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := r.db.Rebind(`SELECT DISTINCT model FROM usage_logs WHERE created_at >= ? AND COALESCE(model, '') <> '' ORDER BY model LIMIT ?`)
	rows, err := r.db.Query(query, since.UTC().Format(sqliteTimestampLayout), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := make([]string, 0, limit)
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		if trimmed := strings.TrimSpace(model); trimmed != "" {
			models = append(models, trimmed)
		}
	}
	return models, rows.Err()
}

// DistinctRecentModelChannels lists the (model, channel) pairs that served
// traffic since the cutoff. The served model depends on the channel's model
// mapping, so billing alignment needs the pair, not just the model.
func (r *Repository) DistinctRecentModelChannels(since time.Time, limit int) ([][2]string, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := r.db.Rebind(`SELECT DISTINCT COALESCE(model, ''), COALESCE(channel, '') FROM usage_logs
		WHERE created_at >= ? AND COALESCE(model, '') <> ''
		ORDER BY 1, 2 LIMIT ?`)
	rows, err := r.db.Query(query, since.UTC().Format(sqliteTimestampLayout), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pairs := make([][2]string, 0, limit)
	for rows.Next() {
		var model, channel string
		if err := rows.Scan(&model, &channel); err != nil {
			return nil, err
		}
		if model != "" {
			pairs = append(pairs, [2]string{model, channel})
		}
	}
	return pairs, rows.Err()
}
