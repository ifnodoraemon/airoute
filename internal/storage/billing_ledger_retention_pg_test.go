//go:build integration

package storage

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

// TestPostgresLedgerArchiveUpsert guards the ON CONFLICT DO UPDATE dialect fix.
// An unqualified cost_units/value is ambiguous between the target row and the
// excluded pseudo-row on PostgreSQL (SQLSTATE 42702), which made every retention
// pass fail and roll back its DELETE. Two ancient orphaned settlements for the
// same key force the per-key archive upsert onto its conflict branch: it must
// accumulate, not error. SQLite cannot reproduce the ambiguity, so this only
// runs against a real PostgreSQL DSN. Assertions are scoped to this run's unique
// key because pruneOnce is a global pass over a possibly persistent database.
func TestPostgresLedgerArchiveUpsert(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := NewRepository(db)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := &UserRecord{Username: "ledger-pg-" + suffix, PasswordHash: "synthetic", Role: "user", Status: "active", Balance: 100}
	if err := r.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &APIKeyRecord{Key: "sk-pg-ledger-" + suffix, UserID: user.ID, TenantID: user.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	ancient := time.Now().UTC().AddDate(0, 0, -400)
	for _, tc := range []struct {
		id    string
		units int64
	}{
		{"bill-pg-a-" + suffix, ToUnits(1.00)},
		{"bill-pg-b-" + suffix, ToUnits(2.00)},
	} {
		if _, err := db.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
			VALUES (?, ?, ?, ?, 0, ?)`, tc.id, key.Key, user.Username, tc.units, ancient); err != nil {
			t.Fatal(err)
		}
	}

	retirer := &BillingLedgerRetention{repo: r, retention: 100 * 24 * time.Hour}
	if _, _, err := retirer.pruneOnce(); err != nil {
		t.Fatalf("PostgreSQL retention pass failed (archive upsert ambiguity?): %v", err)
	}
	var sum int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(cost_units),0) FROM `+billingLedgerArchivesTable+` WHERE api_key = ?`, key.Key).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	if sum != ToUnits(3.00) {
		t.Fatalf("per-key archive = %d units, want %d (conflict branch must accumulate)", sum, ToUnits(3.00))
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE api_key = ?`, key.Key).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d of this key's ledger rows survived retirement", left)
	}
	monthKey := billingLedgerArchiveKeyPrefix + ancient.Format("2006-01")
	raw := r.GetSetting(monthKey, "")
	if raw == "" {
		t.Fatalf("monthly archive key %s was not written", monthKey)
	}
	monthUnits, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("monthly archive key %s = %q, want integer units", monthKey, raw)
	}
	if monthUnits < ToUnits(3.00) {
		t.Fatalf("monthly archive units = %d, want at least %d", monthUnits, ToUnits(3.00))
	}
}