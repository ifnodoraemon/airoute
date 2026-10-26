package storage

// addUsageLogBillingID adds the idempotency key used to make billing writes
// exactly-once.
//
// Before this, a Redis Stream message whose database transaction committed but
// whose XACK did not take effect was redelivered (startup PEL recovery or the
// AutoClaim ticker) and inserted a second usage_logs row *and* a second wallet
// deduction, with no column able to identify the replay afterwards. Redis
// delivery is at-least-once, so persistence must be idempotent.
//
// The column is nullable on purpose: legacy rows keep NULL (a unique index
// permits any number of NULLs in both engines), so this migration needs no
// backfill and cannot fail on pre-existing data.
func addUsageLogBillingID(db *DB) error {
	if db.Dialect() == "postgres" {
		if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS billing_id TEXT`); err != nil {
			return err
		}
	} else if !sqliteColumnExists(db.DB, "usage_logs", "billing_id") {
		if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN billing_id TEXT`); err != nil {
			return err
		}
	}

	// Not partial: NULLs do not collide in a unique index, and every new row
	// gets a generated id.
	_, err := db.DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_usage_billing_id ON usage_logs(billing_id)`)
	return err
}
