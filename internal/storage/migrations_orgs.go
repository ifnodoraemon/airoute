package storage

import "fmt"

// addOrgGovernance introduces the organization model and attributes existing
// rows to it.
//
// Migration 30 is additive only: two new tables, nullable org_id columns, and
// indexes. A root organization ("default", level=unit) is created so every
// legacy user/key/audit row can be attributed without inventing a hierarchy the
// operator did not configure. Attribution never rewrites history: rows keep the
// org they were assigned at request time, and moving an org later only changes
// future attribution.
func addOrgGovernance(db *DB) error {
	orgs := `CREATE TABLE IF NOT EXISTS orgs (
		id BIGSERIAL PRIMARY KEY,
		parent_id BIGINT REFERENCES orgs(id),
		level VARCHAR(16) NOT NULL CHECK (level IN ('unit','department','project')),
		name VARCHAR(128) NOT NULL,
		code VARCHAR(64) NOT NULL UNIQUE,
		path VARCHAR(512) NOT NULL DEFAULT '',
		depth INTEGER NOT NULL DEFAULT 0,
		status VARCHAR(16) NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	)`
	memberships := `CREATE TABLE IF NOT EXISTS org_memberships (
		org_id BIGINT NOT NULL,
		user_id BIGINT NOT NULL,
		role VARCHAR(16) NOT NULL DEFAULT 'member',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (org_id, user_id)
	)`
	if db.Dialect() != "postgres" {
		orgs = `CREATE TABLE IF NOT EXISTS orgs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			parent_id INTEGER REFERENCES orgs(id),
			level TEXT NOT NULL CHECK (level IN ('unit','department','project')),
			name TEXT NOT NULL,
			code TEXT NOT NULL UNIQUE,
			path TEXT NOT NULL DEFAULT '',
			depth INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`
		memberships = `CREATE TABLE IF NOT EXISTS org_memberships (
			org_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (org_id, user_id)
		)`
	}
	for _, statement := range []string{orgs, memberships} {
		if _, err := db.DB.Exec(statement); err != nil {
			return fmt.Errorf("create organization tables: %w", err)
		}
	}

	// An upgrade can start from a legacy/partial schema (older migration fixtures
	// and heavily customized deployments): never ALTER or index a table that is
	// not there. Those tables receive their column when the migration that creates
	// them has run, and the backfill below skips them for the same reason.
	attributionTables := []string{"users", "api_keys", "usage_logs", "billing_settlements"}
	present := make(map[string]bool, len(attributionTables))
	for _, table := range attributionTables {
		exists, err := db.tableExists(table)
		if err != nil {
			return fmt.Errorf("inspect table %s: %w", table, err)
		}
		present[table] = exists
		if exists {
			if err := addOrgIDColumn(db, table); err != nil {
				return err
			}
		}
	}

	indexes := []struct {
		table string
		ddl   string
	}{
		{"orgs", `CREATE INDEX IF NOT EXISTS idx_orgs_parent ON orgs(parent_id)`},
		{"orgs", `CREATE INDEX IF NOT EXISTS idx_orgs_path ON orgs(path)`},
		{"users", `CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id)`},
		{"api_keys", `CREATE INDEX IF NOT EXISTS idx_api_keys_org ON api_keys(org_id)`},
		{"usage_logs", `CREATE INDEX IF NOT EXISTS idx_usage_org_created ON usage_logs(org_id, created_at)`},
		{"billing_settlements", `CREATE INDEX IF NOT EXISTS idx_billing_settlements_org ON billing_settlements(org_id)`},
	}
	for _, index := range indexes {
		if index.table != "orgs" && !present[index.table] {
			continue
		}
		if _, err := db.DB.Exec(index.ddl); err != nil {
			return fmt.Errorf("create organization index: %w", err)
		}
	}

	if err := ensureDefaultOrg(db); err != nil {
		return err
	}
	return backfillOrgAttribution(db, present)
}

// addOrgIDColumn adds a nullable BIGINT/INTEGER column. NULL means "not yet
// attributed"; the backfill fills it, and new rows are written by the request
// path. Nothing reads a NULL as root: that would silently hide a wiring bug.
func addOrgIDColumn(db *DB, table string) error {
	if db.Dialect() == "postgres" {
		if _, err := db.DB.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS org_id BIGINT`, table)); err != nil {
			return fmt.Errorf("add org_id to %s: %w", table, err)
		}
		return nil
	}
	if !sqliteColumnExists(db.DB, table, "org_id") {
		if _, err := db.DB.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN org_id INTEGER`, table)); err != nil {
			return fmt.Errorf("add org_id to %s: %w", table, err)
		}
	}
	return nil
}

// ensureDefaultOrg creates the root unit and fixes its materialized path. The
// insert is idempotent so a crash between DDL and the version record can replay
// safely.
func ensureDefaultOrg(db *DB) error {
	_, err := db.DB.Exec(`INSERT INTO orgs (parent_id, level, name, code, path, depth, status)
		VALUES (NULL, 'unit', '默认单位', 'default', '', 0, 'active')
		ON CONFLICT (code) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("create default organization: %w", err)
	}
	_, err = db.DB.Exec(`UPDATE orgs SET path = '/' || CAST(id AS TEXT) || '/', depth = 0
		WHERE code = 'default' AND (path IS NULL OR path = '')`)
	if err != nil {
		return fmt.Errorf("set default organization path: %w", err)
	}
	return nil
}

// backfillOrgAttribution attributes existing rows to the nearest organization
// that can be derived without inventing data:
//
//	users               -> root unit
//	api_keys            -> owning user's org, else root
//	usage_logs          -> the key's org, else root
//	billing_settlements -> the key's org, else root
//
// The batch loops keep the UPDATE off one giant transaction on a 50k-row audit
// table. Each batch writes a non-NULL value, so progress is monotonic and the
// function is idempotent. Tables absent from the starting schema are skipped, and
// a subquery only runs when its referenced table is present.
func backfillOrgAttribution(db *DB, knownPresent ...map[string]bool) error {
	var present map[string]bool
	if len(knownPresent) > 0 && knownPresent[0] != nil {
		present = knownPresent[0]
	} else {
		// Callers that invoke the backfill directly (for example a migration test)
		// do not have the presence map yet; derive it here.
		present = make(map[string]bool, 4)
		for _, table := range []string{"users", "api_keys", "usage_logs", "billing_settlements"} {
			exists, err := db.tableExists(table)
			if err != nil {
				return fmt.Errorf("inspect table %s: %w", table, err)
			}
			present[table] = exists
		}
	}
	var rootID int64
	if err := db.QueryRow(`SELECT id FROM orgs WHERE code = 'default' LIMIT 1`).Scan(&rootID); err != nil {
		return fmt.Errorf("resolve default organization: %w", err)
	}
	if rootID <= 0 {
		return fmt.Errorf("default organization is missing")
	}

	if present["users"] {
		if err := updateInBatches(db, `UPDATE users SET org_id = ? WHERE org_id IS NULL
			AND id IN (SELECT id FROM users WHERE org_id IS NULL LIMIT 5000)`, rootID); err != nil {
			return fmt.Errorf("backfill users.org_id: %w", err)
		}
	}
	if present["api_keys"] {
		statement := `UPDATE api_keys SET org_id = ? WHERE org_id IS NULL
			AND id IN (SELECT id FROM api_keys WHERE org_id IS NULL LIMIT 5000)`
		if present["users"] {
			statement = `UPDATE api_keys SET org_id = COALESCE(
				(SELECT org_id FROM users WHERE users.id = api_keys.user_id), ?)
				WHERE org_id IS NULL AND id IN (SELECT id FROM api_keys WHERE org_id IS NULL LIMIT 5000)`
		}
		if err := updateInBatches(db, statement, rootID); err != nil {
			return fmt.Errorf("backfill api_keys.org_id: %w", err)
		}
	}
	if present["usage_logs"] {
		statement := `UPDATE usage_logs SET org_id = ? WHERE org_id IS NULL
			AND id IN (SELECT id FROM usage_logs WHERE org_id IS NULL LIMIT 5000)`
		if present["api_keys"] {
			statement = `UPDATE usage_logs SET org_id = COALESCE(
				(SELECT org_id FROM api_keys WHERE api_keys.key = usage_logs.api_key), ?)
				WHERE org_id IS NULL AND id IN (SELECT id FROM usage_logs WHERE org_id IS NULL LIMIT 5000)`
		}
		if err := updateInBatches(db, statement, rootID); err != nil {
			return fmt.Errorf("backfill usage_logs.org_id: %w", err)
		}
	}
	if present["billing_settlements"] {
		statement := `UPDATE billing_settlements SET org_id = ? WHERE org_id IS NULL
			AND id IN (SELECT id FROM billing_settlements WHERE org_id IS NULL LIMIT 5000)`
		if present["api_keys"] {
			statement = `UPDATE billing_settlements SET org_id = COALESCE(
				(SELECT org_id FROM api_keys WHERE api_keys.key = billing_settlements.api_key), ?)
				WHERE org_id IS NULL AND id IN (SELECT id FROM billing_settlements WHERE org_id IS NULL LIMIT 5000)`
		}
		if err := updateInBatches(db, statement, rootID); err != nil {
			return fmt.Errorf("backfill billing_settlements.org_id: %w", err)
		}
	}
	return nil
}

// updateInBatches executes one UPDATE until it affects no more rows. The
// statement must make a NULL column non-NULL, otherwise this would loop forever;
// every caller above satisfies that.
func updateInBatches(db *DB, statement string, args ...any) error {
	for {
		result, err := db.Exec(statement, args...)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
	}
}
