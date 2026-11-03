package storage

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// testPostgresDSNOrSkip keeps the PostgreSQL half of the timestamp tests
// optional: they need a live server, and CI only provides one through this
// variable (a dedicated name, so an exported DATABASE_URL never gets written to).
func testPostgresDSNOrSkip(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("AIROUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIROUTE_TEST_DATABASE_URL is not set; skipping PostgreSQL dialect timestamp test")
	}
	return dsn
}

// E-04/E-05 on PostgreSQL: timestamps must be bound as real time values so the
// session time zone cannot shift them, and a stored verification code must verify
// until its expiry and not after it.
func TestPostgresTimestampBindingAndCodeExpiry(t *testing.T) {
	dsn := testPostgresDSNOrSkip(t)
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	if _, ok := repo.dialectTime(time.Now()).(time.Time); !ok {
		t.Fatalf("dialectTime returned %T on postgres, want time.Time", repo.dialectTime(time.Now()))
	}

	email := fmt.Sprintf("dialect-%d@example.com", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM verification_codes WHERE email = ?`, email)
	})

	// A code valid for ten minutes must verify right away.
	if err := repo.SaveVerificationCode(email, "123456", "register", 10*time.Minute); err != nil {
		t.Fatalf("save code: %v", err)
	}
	if !repo.VerifyCode(email, "123456", "register") {
		t.Fatal("a code valid for ten minutes did not verify (its expiry was shifted by the dialect binding)")
	}

	// A code whose window has already closed must not verify.
	if err := repo.SaveVerificationCode(email, "654321", "register", -time.Minute); err != nil {
		t.Fatalf("save expired code: %v", err)
	}
	if repo.VerifyCode(email, "654321", "register") {
		t.Fatal("an already expired code verified")
	}

	// usage_logs.created_at must round-trip as the instant it was written, not as
	// that instant shifted by the session time zone.
	traceID := fmt.Sprintf("tr-dialect-%d", time.Now().UnixNano())
	writtenAt := time.Now().UTC().Truncate(time.Microsecond)
	rec := &UsageLogRecord{
		TraceID: traceID, Model: "m", StatusCode: 200, Cost: 0.000001, TotalTokens: 1,
		CreatedAt: writtenAt,
	}
	if err := repo.RecordUsageLogContext(t.Context(), rec); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM usage_logs WHERE billing_id = ?`, rec.BillingID)
		_, _ = db.Exec(`DELETE FROM billing_settlements WHERE billing_id = ?`, rec.BillingID)
	})

	var storedAt time.Time
	if err := db.QueryRow(`SELECT created_at FROM usage_logs WHERE billing_id = ?`, rec.BillingID).Scan(&storedAt); err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	if delta := storedAt.Sub(writtenAt); delta > 2*time.Second || delta < -2*time.Second {
		t.Fatalf("usage_logs.created_at = %v, want ~%v (off by %v; the timestamp was bound as zone-less text)", storedAt, writtenAt, delta)
	}
}
