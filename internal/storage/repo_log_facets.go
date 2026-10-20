package storage

// UsageLogFacets describes the caller's entire audit history, independent of
// pagination and active filters. Historical/deleted channels remain searchable.
type UsageLogFacets struct {
	Models   []string `json:"models"`
	Channels []string `json:"channels"`
}

func (r *Repository) ListUsageLogFacets(f LogFilter) (*UsageLogFacets, error) {
	result := &UsageLogFacets{Models: []string{}, Channels: []string{}}
	if f.ScopeByAPIKeys && len(f.APIKeys) == 0 {
		return result, nil
	}
	// Keep the authorization scope, discard search and pagination conditions.
	where, args := r.buildUsageLogWhere(LogFilter{
		TenantID: f.TenantID, APIKeys: f.APIKeys, ScopeByAPIKeys: f.ScopeByAPIKeys,
	})
	for _, field := range []struct {
		column string
		values *[]string
	}{{"model", &result.Models}, {"channel", &result.Channels}} {
		rows, err := r.db.Query(`SELECT DISTINCT `+field.column+` FROM usage_logs WHERE COALESCE(`+field.column+`, '') <> ''`+where+` ORDER BY `+field.column, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			*field.values = append(*field.values, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
