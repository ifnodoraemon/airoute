package storage

// Reservations are money-bearing state. They deliberately have no TTL: an
// unavailable worker must not turn a pending charge into spendable balance.
func createBillingReservations(db *DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS billing_reservations (
		id TEXT PRIMARY KEY, api_key TEXT NOT NULL, user_id BIGINT NOT NULL,
		amount_units BIGINT NOT NULL CHECK (amount_units >= 0),
		billing_id TEXT NOT NULL DEFAULT '', pending BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_billing_reservations_user ON billing_reservations(user_id);
	CREATE INDEX IF NOT EXISTS idx_billing_reservations_key ON billing_reservations(api_key);`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS billing_usage_journal (
		billing_id TEXT PRIMARY KEY, payload TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}
