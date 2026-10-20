package storage

import "fmt"

// MoneyDrift describes a mismatch between the authoritative integer ledger and
// the legacy floating-point mirror for a single API key.
type MoneyDrift struct {
	APIKey        string  `json:"api_key"`
	LogCostUnits  int64   `json:"log_cost_units"`   // SUM(usage_logs.cost_units)
	KeyCostUnits  int64   `json:"key_cost_units"`   // api_keys.used_cost_units
	Difference    int64   `json:"difference_units"` // log - key
	DifferenceCNY float64 `json:"difference_cny"`
}

// ReconcileMoney compares per-key integer cost ledgers and reports drift. It is
// meant to be run periodically after the dual-write rollout to prove that the
// legacy REAL columns and the integer mirrors agree.
func (r *Repository) ReconcileMoney(limit int) ([]*MoneyDrift, error) {
	if limit <= 0 {
		limit = 100
	}
	query := r.db.Rebind(`
		SELECT k.key,
		       COALESCE(SUM(l.cost_units), 0) AS log_cost,
		       COALESCE(k.used_cost_units, 0) AS key_cost
		FROM api_keys k
		LEFT JOIN usage_logs l ON l.api_key = k.key
		GROUP BY k.key, k.used_cost_units
		HAVING COALESCE(SUM(l.cost_units), 0) <> COALESCE(k.used_cost_units, 0)
		LIMIT ?`)
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("reconcile money query: %w", err)
	}
	defer rows.Close()

	var out []*MoneyDrift
	for rows.Next() {
		var d MoneyDrift
		if err := rows.Scan(&d.APIKey, &d.LogCostUnits, &d.KeyCostUnits); err != nil {
			return nil, err
		}
		d.Difference = d.LogCostUnits - d.KeyCostUnits
		d.DifferenceCNY = FromUnits(d.Difference)
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
