package storage

import (
	"context"
	"strings"
	"testing"
)

// TestNewRowsCarryOrgAttribution pins the write-side half of migration 30: the
// backfill fixes history, and these paths must not create new NULL rows.
// Without them a restart's backfill would silently "repair" attribution that was
// missing from the start, hiding the wiring bug.
func TestNewRowsCarryOrgAttribution(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)
	ctx := context.Background()

	var rootID int64
	if err := db.QueryRow(`SELECT id FROM orgs WHERE code = 'default'`).Scan(&rootID); err != nil {
		t.Fatalf("default org: %v", err)
	}

	user := &UserRecord{Username: "org-attrib-user", PasswordHash: "x"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	var userOrg int64
	if err := db.QueryRow(`SELECT org_id FROM users WHERE id = ?`, user.ID).Scan(&userOrg); err != nil {
		t.Fatalf("read user org: %v", err)
	}
	if userOrg != rootID {
		t.Fatalf("new user org = %d, want default %d", userOrg, rootID)
	}

	key := &APIKeyRecord{
		Key:      strings.Repeat("d", 64),
		TenantID: user.Username,
		UserID:   user.ID,
	}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if key.OrgID != rootID {
		t.Fatalf("key.OrgID = %d, want %d", key.OrgID, rootID)
	}
	var keyOrg int64
	if err := db.QueryRow(`SELECT org_id FROM api_keys WHERE id = ?`, key.ID).Scan(&keyOrg); err != nil {
		t.Fatalf("read key org: %v", err)
	}
	if keyOrg != rootID {
		t.Fatalf("new key org = %d, want default %d", keyOrg, rootID)
	}

	loaded, err := repo.GetAPIKeyByKeyContext(ctx, key.Key)
	if err != nil {
		t.Fatalf("load key context: %v", err)
	}
	if loaded.OrgID != rootID {
		t.Fatalf("authenticated key org = %d, want %d", loaded.OrgID, rootID)
	}

	// Audit path.
	audit := &UsageLogRecord{
		TraceID:     "tr-org-attrib",
		APIKey:      key.Key,
		TenantID:    user.Username,
		OrgID:       rootID,
		Model:       "m",
		Channel:     "c",
		TotalTokens: 1,
		BillingID:   "bill-org-attrib",
	}
	if err := repo.insertUsageLogsContext(ctx, []*UsageLogRecord{audit}); err != nil {
		t.Fatalf("insert usage log: %v", err)
	}
	var auditOrg int64
	if err := db.QueryRow(`SELECT org_id FROM usage_logs WHERE trace_id = ?`, audit.TraceID).Scan(&auditOrg); err != nil {
		t.Fatalf("read audit org: %v", err)
	}
	if auditOrg != rootID {
		t.Fatalf("audit org = %d, want %d", auditOrg, rootID)
	}

	// Settlement ledger path.
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin settlement tx: %v", err)
	}
	settled, err := repo.insertSettlementRows(tx, []*UsageLogRecord{{
		BillingID:   "bill-org-attrib-settle",
		APIKey:      key.Key,
		TenantID:    user.Username,
		OrgID:       rootID,
		Cost:        0.01,
		TotalTokens: 1,
	}})
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert settlement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit settlement: %v", err)
	}
	if _, ok := settled["bill-org-attrib-settle"]; !ok {
		t.Fatalf("settlement was not inserted: %v", settled)
	}
	var settlementOrg int64
	if err := db.QueryRow(`SELECT org_id FROM billing_settlements WHERE billing_id = ?`, "bill-org-attrib-settle").Scan(&settlementOrg); err != nil {
		t.Fatalf("read settlement org: %v", err)
	}
	if settlementOrg != rootID {
		t.Fatalf("settlement org = %d, want %d", settlementOrg, rootID)
	}
}

// A user with an explicit organization must not be overwritten by the default.
func TestKeyInheritsOwnerSpecificOrg(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	defaultID, err := repo.DefaultOrgID()
	if err != nil {
		t.Fatal(err)
	}
	unit := &OrgRecord{Level: OrgLevelUnit, Name: "Specific Unit", Code: "specific-unit"}
	if err := repo.CreateOrg(unit); err != nil {
		t.Fatalf("create unit: %v", err)
	}
	user := &UserRecord{Username: "specific-org-user", PasswordHash: "x"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`UPDATE users SET org_id = ? WHERE id = ?`, unit.ID, user.ID); err != nil {
		t.Fatalf("attach user to unit: %v", err)
	}

	key := &APIKeyRecord{Key: strings.Repeat("e", 64), TenantID: user.Username, UserID: user.ID}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if key.OrgID != unit.ID {
		t.Fatalf("key org = %d, want explicit unit %d (default was %d)", key.OrgID, unit.ID, defaultID)
	}
}
