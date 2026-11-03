package storage

import (
	"context"
	"testing"
)

// legacyGovernanceDDL recreates exactly what the cut v1.1 slice used to add, so
// the cleanup migration is exercised against a database shaped like an upgraded
// one (a fresh database never contains any of it).
var legacyGovernanceDDL = []string{
	`CREATE TABLE orgs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		parent_id INTEGER REFERENCES orgs(id),
		level TEXT NOT NULL,
		name TEXT NOT NULL,
		code TEXT NOT NULL UNIQUE,
		path TEXT NOT NULL DEFAULT '',
		depth INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX idx_orgs_parent ON orgs(parent_id)`,
	`CREATE INDEX idx_orgs_path ON orgs(path)`,
	`INSERT INTO orgs (level, name, code, path) VALUES ('unit', '默认单位', 'default', '/1/')`,
	`CREATE TABLE org_memberships (
		org_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		role TEXT NOT NULL DEFAULT 'member',
		PRIMARY KEY (org_id, user_id)
	)`,
	`CREATE TABLE budgets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scope_type TEXT NOT NULL,
		scope_id INTEGER NOT NULL,
		period TEXT NOT NULL,
		amount_units INTEGER NOT NULL,
		used_units INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE budget_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		budget_id INTEGER NOT NULL,
		threshold INTEGER NOT NULL,
		used_units INTEGER NOT NULL,
		amount_units INTEGER NOT NULL
	)`,
	`CREATE TABLE wallet_transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		type TEXT NOT NULL,
		delta_units INTEGER NOT NULL,
		balance_after_units INTEGER NOT NULL,
		ref_id TEXT
	)`,
	`CREATE TABLE usage_daily (
		day TEXT NOT NULL,
		org_id INTEGER NOT NULL DEFAULT 0,
		model TEXT NOT NULL,
		channel TEXT NOT NULL,
		PRIMARY KEY (day, org_id, model, channel)
	)`,
	`ALTER TABLE users ADD COLUMN org_id INTEGER`,
	`ALTER TABLE api_keys ADD COLUMN org_id INTEGER`,
	`ALTER TABLE usage_logs ADD COLUMN org_id INTEGER`,
	`ALTER TABLE usage_logs ADD COLUMN cost_basis_units INTEGER`,
	`ALTER TABLE usage_logs ADD COLUMN channel_id INTEGER`,
	`ALTER TABLE billing_settlements ADD COLUMN org_id INTEGER`,
	`CREATE INDEX idx_users_org ON users(org_id)`,
	`CREATE INDEX idx_api_keys_org ON api_keys(org_id)`,
	`CREATE INDEX idx_usage_org_created ON usage_logs(org_id, created_at)`,
	`CREATE INDEX idx_usage_logs_channel_id ON usage_logs(channel_id)`,
	`CREATE INDEX idx_billing_settlements_org ON billing_settlements(org_id)`,
}

func seedLegacyGovernance(t *testing.T, db *DB, repo *Repository) {
	t.Helper()
	for _, statement := range legacyGovernanceDDL {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed legacy schema (%s): %v", statement, err)
		}
	}
	if err := repo.CreateUser(&UserRecord{Username: "legacy-user", Status: "active", Role: "user"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET org_id = 1 WHERE username = 'legacy-user'`); err != nil {
		t.Fatalf("tag user org: %v", err)
	}
	if err := repo.CreateAPIKey(&APIKeyRecord{Key: "sk-legacy", TenantID: "legacy-user", Status: "active"}); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	if err := repo.insertUsageLogsContext(context.Background(), []*UsageLogRecord{
		{APIKey: "sk-legacy", Model: "legacy-model", TotalTokens: 3, StatusCode: 200},
	}); err != nil {
		t.Fatalf("seed usage log: %v", err)
	}
	if _, err := db.Exec(`UPDATE usage_logs SET org_id = 1, cost_basis_units = 7, channel_id = 2`); err != nil {
		t.Fatalf("tag usage log cost fields: %v", err)
	}
}

// TestDropLegacyGovernanceObjectsOnLegacyDatabase is the upgrade path: the dead
// tables and columns must be gone afterwards while the audit rows themselves
// stay usable.
func TestDropLegacyGovernanceObjectsOnLegacyDatabase(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)
	seedLegacyGovernance(t, db, repo)

	if err := dropLegacyGovernanceObjects(db); err != nil {
		t.Fatalf("cleanup migration: %v", err)
	}

	for _, table := range []string{"orgs", "org_memberships", "budgets", "budget_events", "wallet_transactions", "usage_daily"} {
		var found string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found)
		if err == nil {
			t.Fatalf("table %s survived the cleanup migration", table)
		}
	}
	for _, column := range []struct {
		table  string
		column string
	}{
		{"billing_settlements", "org_id"},
		{"users", "org_id"},
		{"api_keys", "org_id"},
		{"usage_logs", "org_id"},
		{"usage_logs", "cost_basis_units"},
		{"usage_logs", "channel_id"},
	} {
		if sqliteColumnExists(db.DB, column.table, column.column) {
			t.Fatalf("column %s.%s survived the cleanup migration", column.table, column.column)
		}
	}

	// The audit rows are untouched: only the cut columns/tables are removed.
	var users, keys, logs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&keys); err != nil {
		t.Fatalf("count keys: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&logs); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if users != 1 || keys != 1 || logs != 1 {
		t.Fatalf("rows lost: users=%d keys=%d logs=%d", users, keys, logs)
	}

	// The rewritten tables must still accept writes (the ledger path writes audit
	// rows and settlements after every request).
	if err := repo.insertUsageLogsContext(context.Background(), []*UsageLogRecord{
		{APIKey: "sk-legacy", Model: "after-cleanup", TotalTokens: 1, StatusCode: 200},
	}); err != nil {
		t.Fatalf("write after cleanup: %v", err)
	}

	// A crash between DDL and the version record leaves the migration replayable.
	if err := dropLegacyGovernanceObjects(db); err != nil {
		t.Fatalf("replay cleanup migration: %v", err)
	}
}

// TestDropLegacyGovernanceObjectsOnFreshDatabase pins the no-op case: a database
// created by the current binaries has none of the legacy objects.
func TestDropLegacyGovernanceObjectsOnFreshDatabase(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := dropLegacyGovernanceObjects(db); err != nil {
		t.Fatalf("cleanup migration on fresh db: %v", err)
	}
	if sqliteColumnExists(db.DB, "usage_logs", "org_id") {
		t.Fatal("fresh database should not have usage_logs.org_id")
	}
}
