package storage

import (
	"strings"
	"testing"
)

// billingScaleIndexExists reports whether a named index exists in the SQLite
// schema.
func billingScaleIndexExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("look up index %s: %v", name, err)
	}
	return count > 0
}

// billingScaleTableExists reports whether a named table exists in the SQLite
// schema.
func billingScaleTableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("look up table %s: %v", name, err)
	}
	return count > 0
}

// billingScaleQueryPlan returns the flattened EXPLAIN QUERY PLAN output.
func billingScaleQueryPlan(t *testing.T, db *DB, query string) string {
	t.Helper()
	rows, err := db.Query(`EXPLAIN QUERY PLAN ` + query)
	if err != nil {
		t.Fatalf("explain %q: %v", query, err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read plan: %v", err)
	}
	return plan.String()
}

// TestBillingScaleIndexesMigration pins migration 41: the reconciliation index is
// created, the archive aggregate exists, and the index is what the reconciliation
// aggregate actually uses (otherwise the migration would be pointless).
func TestBillingScaleIndexesMigration(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var name string
	if err := db.QueryRow(`SELECT name FROM schema_migrations WHERE version = 41`).Scan(&name); err != nil {
		t.Fatalf("migration 41 is not recorded: %v", err)
	}
	if name != "billing_scale_indexes" {
		t.Fatalf("migration 41 name = %q, want billing_scale_indexes", name)
	}

	if !billingScaleIndexExists(t, db, billingSettlementsAPIKeyIndex) {
		t.Fatalf("index %s was not created", billingSettlementsAPIKeyIndex)
	}
	if !billingScaleTableExists(t, db, billingLedgerArchivesTable) {
		t.Fatalf("table %s was not created", billingLedgerArchivesTable)
	}

	// The whole point of the composite index: per-key settlement totals are the
	// hot shape of ledgerDrift.
	plan := billingScaleQueryPlan(t, db, `SELECT api_key, SUM(cost_units) FROM billing_settlements GROUP BY api_key`)
	if !strings.Contains(plan, billingSettlementsAPIKeyIndex) {
		t.Fatalf("ledgerDrift aggregate does not use %s:\n%s", billingSettlementsAPIKeyIndex, plan)
	}
}

// TestBillingScaleIndexesDropsUnusedTrigramIndexes covers the index diet: the five
// trigram indexes that pg_stat_user_indexes showed as unused are dropped, the
// error_message one is kept, and a replay is a no-op.
func TestBillingScaleIndexesDropsUnusedTrigramIndexes(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate the PostgreSQL deployment: all six trigram indexes exist. SQLite
	// never got them (addUsageLogSearchIndexes returns early for non-postgres), so
	// create plain stand-ins with the production names.
	for _, name := range append(append([]string{}, unusedUsageTrigramIndexes...), keptUsageTrigramIndex) {
		if _, err := db.Exec(`CREATE INDEX ` + name + ` ON usage_logs(model)`); err != nil {
			t.Fatalf("create stand-in %s: %v", name, err)
		}
	}

	for pass := 0; pass < 2; pass++ {
		if err := addBillingScaleIndexes(db); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	for _, name := range unusedUsageTrigramIndexes {
		if billingScaleIndexExists(t, db, name) {
			t.Fatalf("%s survived the migration", name)
		}
	}
	if !billingScaleIndexExists(t, db, keptUsageTrigramIndex) {
		t.Fatalf("%s must be kept: it is the one trigram index that still gets scanned", keptUsageTrigramIndex)
	}
	if !billingScaleIndexExists(t, db, billingSettlementsAPIKeyIndex) {
		t.Fatalf("%s is missing after the replay", billingSettlementsAPIKeyIndex)
	}
}

// TestBillingScaleIndexesMigrationReplay proves the migration is idempotent on a
// database that already has the index and the archive table: dropping the version
// record and replaying the whole list must not error.
func TestBillingScaleIndexesMigrationReplay(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 41`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrate(); err != nil {
		t.Fatalf("replaying migrations after removing version 41: %v", err)
	}

	var recorded int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 41`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("migration 41 recorded %d times, want 1", recorded)
	}
	if !billingScaleIndexExists(t, db, billingSettlementsAPIKeyIndex) {
		t.Fatalf("%s is missing after the replay", billingSettlementsAPIKeyIndex)
	}
}

// TestKeptTrigramIndexIsNotInTheDropList guards against a later "cleanup" that
// decides all trigram indexes are equally useless.
func TestKeptTrigramIndexIsNotInTheDropList(t *testing.T) {
	for _, name := range unusedUsageTrigramIndexes {
		if name == keptUsageTrigramIndex {
			t.Fatalf("%s must not be in unusedUsageTrigramIndexes", keptUsageTrigramIndex)
		}
	}
}
