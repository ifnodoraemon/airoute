package storage

import "fmt"

func addPaymentLifecycle(db *DB) error {
	// This migration owns its base table and can be replayed independently.
	id := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if db.Dialect() == "postgres" {
		id = "BIGSERIAL PRIMARY KEY"
	}
	if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS recharge_orders (
		id %s, order_no TEXT UNIQUE NOT NULL, username TEXT NOT NULL,
		amount DOUBLE PRECISION NOT NULL DEFAULT 0, currency TEXT NOT NULL DEFAULT 'CNY',
		channel TEXT NOT NULL DEFAULT 'stripe', stripe_session_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`, id)); err != nil {
		return err
	}
	columns := []struct{ name, definition string }{
		{"idempotency_key", "TEXT NOT NULL DEFAULT ''"},
		{"payment_mode", "TEXT NOT NULL DEFAULT 'live'"},
		{"checkout_url", "TEXT NOT NULL DEFAULT ''"},
		{"success_url", "TEXT NOT NULL DEFAULT ''"},
		{"cancel_url", "TEXT NOT NULL DEFAULT ''"},
		{"expires_at", "BIGINT NOT NULL DEFAULT 0"},
		{"payment_intent_id", "TEXT NOT NULL DEFAULT ''"},
		{"credited_cents", "BIGINT NOT NULL DEFAULT 0"},
		{"reversed_cents", "BIGINT NOT NULL DEFAULT 0"},
		{"refunded_cents", "BIGINT NOT NULL DEFAULT 0"},
		{"refund_pending_cents", "BIGINT NOT NULL DEFAULT 0"},
		{"disputed_cents", "BIGINT NOT NULL DEFAULT 0"},
		{"next_reconcile_at", "BIGINT NOT NULL DEFAULT 0"},
		{"sync_version", "BIGINT NOT NULL DEFAULT 0"},
		{"lease_until", "BIGINT NOT NULL DEFAULT 0"},
		{"last_sync_error", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, col := range columns {
		if db.Dialect() == "sqlite" && sqliteColumnExists(db.DB, "recharge_orders", col.name) {
			continue
		}
		optional := ""
		if db.Dialect() == "postgres" {
			optional = "IF NOT EXISTS "
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE recharge_orders ADD COLUMN %s%s %s", optional, col.name, col.definition)); err != nil {
			return err
		}
	}
	_, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_recharge_intent ON recharge_orders(username, idempotency_key) WHERE idempotency_key <> '';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_recharge_session ON recharge_orders(stripe_session_id) WHERE stripe_session_id <> '';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_recharge_payment_intent ON recharge_orders(payment_intent_id) WHERE payment_intent_id <> '';
		CREATE INDEX IF NOT EXISTS idx_recharge_reconcile ON recharge_orders(payment_mode, next_reconcile_at);
		CREATE TABLE IF NOT EXISTS payment_adjustments (
			kind TEXT NOT NULL, object_id TEXT NOT NULL, order_no TEXT NOT NULL,
			amount_minor BIGINT NOT NULL, currency TEXT NOT NULL, wallet_cents BIGINT NOT NULL, status TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(kind, object_id)
		);
		CREATE INDEX IF NOT EXISTS idx_payment_adjustment_order ON payment_adjustments(order_no);
		CREATE TABLE IF NOT EXISTS stripe_events (
			id TEXT PRIMARY KEY, event_type TEXT NOT NULL, object_id TEXT NOT NULL,
			payment_mode TEXT NOT NULL,
			processed INTEGER NOT NULL DEFAULT 0, attempts INTEGER NOT NULL DEFAULT 0,
			next_attempt_at BIGINT NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_stripe_events_pending ON stripe_events(payment_mode, processed, next_attempt_at);
	`)
	return err
}
