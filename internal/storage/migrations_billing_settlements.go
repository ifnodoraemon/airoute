package storage

// addBillingSettlements creates the ledger that records which charges have
// already been applied to a wallet.
//
// Before this table, the wallet movement lived inside the same transaction as
// the audit row, so a charge existed only if the audit write succeeded. The
// ledger makes the money side independently idempotent: a charge is applied when
// its billing_id is inserted here, and a redelivery of the same record finds the
// row already present and moves nothing.
//
// Existing usage rows are backfilled so the ledger starts in agreement with the
// audit trail (including the historical rows that predate billing_id, which get a
// deterministic "legacy-<id>" key).
func addBillingSettlements(db *DB) error {
	create := `CREATE TABLE IF NOT EXISTS billing_settlements (
		id BIGSERIAL PRIMARY KEY,
		billing_id TEXT NOT NULL,
		api_key VARCHAR(255) DEFAULT '',
		tenant_id VARCHAR(128) DEFAULT '',
		cost_units BIGINT NOT NULL DEFAULT 0,
		total_tokens BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	)`
	if db.Dialect() != "postgres" {
		create = `CREATE TABLE IF NOT EXISTS billing_settlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			billing_id TEXT NOT NULL,
			api_key VARCHAR(255) DEFAULT '',
			tenant_id VARCHAR(128) DEFAULT '',
			cost_units INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`
	}
	if _, err := db.DB.Exec(create); err != nil {
		return err
	}
	if _, err := db.DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_settlements_billing_id ON billing_settlements(billing_id)`); err != nil {
		return err
	}
	if _, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_billing_settlements_created ON billing_settlements(created_at)`); err != nil {
		return err
	}

	// Backfill rows that already carry a billing_id.
	backfillModern := `INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
		SELECT billing_id, COALESCE(api_key, ''), COALESCE(tenant_id, ''), cost_units, total_tokens, created_at
		FROM usage_logs
		WHERE billing_id IS NOT NULL AND billing_id <> ''
		ON CONFLICT (billing_id) DO NOTHING`
	if _, err := db.DB.Exec(backfillModern); err != nil {
		return err
	}

	// Legacy rows have no billing_id; derive a stable key from the primary key.
	backfillLegacy := `INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
		SELECT 'legacy-' || id, COALESCE(api_key, ''), COALESCE(tenant_id, ''), cost_units, total_tokens, created_at
		FROM usage_logs
		WHERE (billing_id IS NULL OR billing_id = '')
		ON CONFLICT (billing_id) DO NOTHING`
	if _, err := db.DB.Exec(backfillLegacy); err != nil {
		return err
	}
	return nil
}
