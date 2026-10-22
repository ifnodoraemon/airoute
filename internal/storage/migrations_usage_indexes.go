package storage

// addUsageLogQueryIndexes adds composite indexes for the console's filtered
// queries. The single-column indexes cannot serve a scoped filter combined with
// a created_at range, which is exactly the shape of every page load: tenant, key
// or model equality plus a time window.
func addUsageLogQueryIndexes(db *DB) error {
	for _, index := range []struct {
		name    string
		columns string
	}{
		{"idx_usage_tenant_created", "tenant_id, created_at"},
		{"idx_usage_key_created", "api_key, created_at"},
		{"idx_usage_model_created", "model, created_at"},
	} {
		if _, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS ` + index.name + ` ON usage_logs(` + index.columns + `)`); err != nil {
			return err
		}
	}
	return nil
}
