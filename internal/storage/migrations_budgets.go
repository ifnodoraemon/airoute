package storage

// addBudgets creates the v1.1 budget tables.
//
// It is deliberately not registered in the migration list yet: another session
// was editing migrations_sqlite.go/migrations_postgres.go while this slice was
// written, and adding the version line there would have collided. Once that
// change lands, registering version 31 is a two-line change; until then the
// repository tests call this function directly.
func addBudgets(db *DB) error {
	budgets := `CREATE TABLE IF NOT EXISTS budgets (
		id BIGSERIAL PRIMARY KEY,
		scope_type VARCHAR(16) NOT NULL,
		scope_id BIGINT NOT NULL,
		period VARCHAR(16) NOT NULL,
		amount_units BIGINT NOT NULL,
		used_units BIGINT NOT NULL DEFAULT 0,
		action VARCHAR(16) NOT NULL DEFAULT 'notify',
		warn_percent INTEGER NOT NULL DEFAULT 80,
		status VARCHAR(16) NOT NULL DEFAULT 'active',
		period_start TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (scope_type, scope_id, period)
	)`
	events := `CREATE TABLE IF NOT EXISTS budget_events (
		id BIGSERIAL PRIMARY KEY,
		budget_id BIGINT NOT NULL,
		threshold INTEGER NOT NULL,
		period_key VARCHAR(32) NOT NULL DEFAULT '',
		used_units BIGINT NOT NULL DEFAULT 0,
		amount_units BIGINT NOT NULL DEFAULT 0,
		action VARCHAR(16) NOT NULL DEFAULT 'notify',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (budget_id, threshold, period_key)
	)`
	if db.Dialect() != "postgres" {
		budgets = `CREATE TABLE IF NOT EXISTS budgets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope_type TEXT NOT NULL,
			scope_id INTEGER NOT NULL,
			period TEXT NOT NULL,
			amount_units INTEGER NOT NULL,
			used_units INTEGER NOT NULL DEFAULT 0,
			action TEXT NOT NULL DEFAULT 'notify',
			warn_percent INTEGER NOT NULL DEFAULT 80,
			status TEXT NOT NULL DEFAULT 'active',
			period_start DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (scope_type, scope_id, period)
		)`
		events = `CREATE TABLE IF NOT EXISTS budget_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			budget_id INTEGER NOT NULL,
			threshold INTEGER NOT NULL,
			period_key TEXT NOT NULL DEFAULT '',
			used_units INTEGER NOT NULL DEFAULT 0,
			amount_units INTEGER NOT NULL DEFAULT 0,
			action TEXT NOT NULL DEFAULT 'notify',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (budget_id, threshold, period_key)
		)`
	}
	for _, statement := range []string{budgets, events} {
		if _, err := db.DB.Exec(statement); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_budgets_scope ON budgets(scope_type, scope_id)`,
		`CREATE INDEX IF NOT EXISTS idx_budget_events_budget_created ON budget_events(budget_id, created_at)`,
	} {
		if _, err := db.DB.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}
