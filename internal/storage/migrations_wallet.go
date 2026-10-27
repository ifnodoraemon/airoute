package storage

// addWalletLedger creates the append-only wallet ledger. It is exported through
// CreateWalletLedger so the migration registration (blocked by a concurrent
// migration-list edit) and tests can call it directly.
func addWalletLedger(db *DB) error {
	table := `CREATE TABLE IF NOT EXISTS wallet_transactions (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL,
		type VARCHAR(24) NOT NULL,
		delta_units BIGINT NOT NULL,
		balance_after_units BIGINT NOT NULL,
		operator VARCHAR(64) NOT NULL DEFAULT '',
		reason VARCHAR(255) NOT NULL DEFAULT '',
		ref_id VARCHAR(128) NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	)`
	if db.Dialect() != "postgres" {
		table = `CREATE TABLE IF NOT EXISTS wallet_transactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			delta_units INTEGER NOT NULL,
			balance_after_units INTEGER NOT NULL,
			operator TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			ref_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`
	}
	if _, err := db.DB.Exec(table); err != nil {
		return err
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_wallet_tx_user_created ON wallet_transactions(user_id, created_at)`,
		// A non-empty ref makes the write idempotent (recharge webhooks, retried
		// settlements). Empty refs are allowed for manual adjustments.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_tx_ref ON wallet_transactions(user_id, type, ref_id) WHERE ref_id <> ''`,
	} {
		if _, err := db.DB.Exec(statement); err != nil {
			return err
		}
	}
	// Opening balance: one row per existing user so the ledger sum starts at the
	// current wallet balance. Idempotent via NOT EXISTS, safe on replay.
	backfill := `INSERT INTO wallet_transactions (user_id, type, delta_units, balance_after_units, operator, reason, ref_id)
		SELECT u.id, 'opening', COALESCE(u.balance_units, 0), COALESCE(u.balance_units, 0), 'system', 'opening balance', 'opening:' || u.id
		FROM users u
		WHERE NOT EXISTS (
			SELECT 1 FROM wallet_transactions w WHERE w.user_id = u.id AND w.type = 'opening'
		)`
	if _, err := db.DB.Exec(backfill); err != nil {
		return err
	}
	return nil
}

// CreateWalletLedger is the exported schema entrypoint used by the migration
// registration once the concurrent migration-list edit is committed.
func CreateWalletLedger(db *DB) error {
	if err := addWalletLedger(db); err != nil {
		return err
	}
	markWalletLedgerReady(db)
	return nil
}
