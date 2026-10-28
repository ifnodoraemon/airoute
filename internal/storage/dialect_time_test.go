package storage

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func newDialectRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "dialect.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

// E-04: on SQLite every timestamp is bound as fixed-width UTC text, and the
// precision differs per column family (usage_logs/ledger carry microseconds,
// everything else whole seconds).
func TestDialectTimeBindsSQLiteText(t *testing.T) {
	repo := newDialectRepo(t)
	at := time.Date(2026, 10, 28, 12, 34, 56, 123456000, time.UTC)

	got, ok := repo.dialectTime(at).(string)
	if !ok {
		t.Fatalf("dialectTime returned %T, want string on sqlite", repo.dialectTime(at))
	}
	if want := "2026-10-28 12:34:56"; got != want {
		t.Fatalf("dialectTime = %q, want %q", got, want)
	}

	precise, ok := repo.dialectTimePrecise(at).(string)
	if !ok {
		t.Fatalf("dialectTimePrecise returned %T, want string on sqlite", repo.dialectTimePrecise(at))
	}
	if want := "2026-10-28 12:34:56.123456"; precise != want {
		t.Fatalf("dialectTimePrecise = %q, want %q", precise, want)
	}
}

// E-04: usage_logs and the settlement ledger are written with the dialect helper,
// so the stored text keeps the microsecond layout the read filters parse.
func TestUsageAndLedgerTimestampsKeepMicrosecondLayout(t *testing.T) {
	repo := newDialectRepo(t)
	want := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Second)

	rec := &UsageLogRecord{
		TraceID: "tr-precise", APIKey: "digest-not-a-key", Model: "m", StatusCode: 200,
		Cost: 0.25, TotalTokens: 10, CreatedAt: want,
	}
	if err := repo.RecordUsageLogContext(t.Context(), rec); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	var usageAt string
	if err := repo.db.QueryRow(`SELECT CAST(created_at AS TEXT) FROM usage_logs WHERE billing_id = ?`, rec.BillingID).Scan(&usageAt); err != nil {
		t.Fatalf("read usage created_at: %v", err)
	}
	parsed, err := time.Parse(sqliteTimestampLayout, usageAt)
	if err != nil {
		t.Fatalf("usage created_at %q is not in the microsecond layout: %v", usageAt, err)
	}
	if !parsed.Equal(want.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("usage created_at = %v, want %v", parsed, want)
	}

	var ledgerAt string
	if err := repo.db.QueryRow(`SELECT CAST(created_at AS TEXT) FROM billing_settlements WHERE billing_id = ?`, rec.BillingID).Scan(&ledgerAt); err != nil {
		t.Fatalf("read ledger created_at: %v", err)
	}
	parsedLedger, err := time.Parse(sqliteTimestampLayout, ledgerAt)
	if err != nil {
		t.Fatalf("ledger created_at %q is not in the microsecond layout: %v", ledgerAt, err)
	}
	if !parsedLedger.Equal(want.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("ledger created_at = %v, want %v", parsedLedger, want)
	}
}

// E-05 regression: a verification code is written through the same dialect
// binding its expiry check uses, so "not expired yet" and "expired" agree.
func TestVerificationCodeExpiryUsesDialectBinding(t *testing.T) {
	repo := newDialectRepo(t)

	if err := repo.SaveVerificationCode("Expiry@Example.com", "123456", "register", 10*time.Minute); err != nil {
		t.Fatalf("save code: %v", err)
	}
	var stored string
	if err := repo.db.QueryRow(`SELECT CAST(expires_at AS TEXT) FROM verification_codes WHERE email = ?`, "expiry@example.com").Scan(&stored); err != nil {
		t.Fatalf("read expires_at: %v", err)
	}
	if _, err := time.Parse(legacySecondTimestampLayout, stored); err != nil {
		t.Fatalf("expires_at %q is not in the second-precision text layout: %v", stored, err)
	}
	if ok := repo.VerifyCode("expiry@example.com", "123456", "register"); !ok {
		t.Fatal("a freshly stored code must verify")
	}

	// An already-expired row must be rejected even though it is still `used = 0`.
	if err := repo.SaveVerificationCode("stale@example.com", "654321", "register", time.Hour); err != nil {
		t.Fatalf("save stale code: %v", err)
	}
	if _, err := repo.db.Exec(`UPDATE verification_codes SET expires_at = ? WHERE email = ?`,
		repo.dialectTime(time.Now().Add(-time.Hour)), "stale@example.com"); err != nil {
		t.Fatalf("backdate code: %v", err)
	}
	if ok := repo.VerifyCode("stale@example.com", "654321", "register"); ok {
		t.Fatal("an expired code must not verify")
	}
}

// E-06: the video_tasks DDL really is dialect-specific (zone-aware on PostgreSQL,
// TEXT/DATETIME on SQLite), instead of one DDL for both engines.
func TestVideoTasksDDLIsDialectSpecific(t *testing.T) {
	postgres := videoTasksDDL("postgres")
	if !strings.Contains(postgres, "created_at TIMESTAMPTZ") {
		t.Fatalf("postgres DDL lost TIMESTAMPTZ: %s", postgres)
	}
	sqlite := videoTasksDDL("sqlite")
	if !strings.Contains(sqlite, "created_at DATETIME") {
		t.Fatalf("sqlite DDL lost DATETIME: %s", sqlite)
	}
	if strings.Contains(sqlite, "TIMESTAMPTZ") {
		t.Fatalf("sqlite DDL must not use TIMESTAMPTZ: %s", sqlite)
	}
}

// A-11 regression: the fallback trace id is unique per request, never the session
// id, and keeps the documented shape.
func TestEnsureTraceIDNeverBorrowsSessionID(t *testing.T) {
	format := regexp.MustCompile(`^tr-\d+-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		rec := &UsageLogRecord{SessionID: "session-shared-by-many-requests"}
		ensureTraceID(rec)
		if rec.TraceID == rec.SessionID {
			t.Fatal("trace id fell back to the session id")
		}
		if !format.MatchString(rec.TraceID) {
			t.Fatalf("fallback trace id %q does not match tr-<micro>-<12 hex>", rec.TraceID)
		}
	}

	// An existing id (a real traceparent/middleware value) is never replaced.
	rec := &UsageLogRecord{TraceID: "tr-from-middleware", SessionID: "sess"}
	ensureTraceID(rec)
	if rec.TraceID != "tr-from-middleware" {
		t.Fatalf("ensureTraceID replaced an existing id with %q", rec.TraceID)
	}

	// Two requests in the same session must not collide.
	a := &UsageLogRecord{SessionID: "same-session"}
	b := &UsageLogRecord{SessionID: "same-session"}
	ensureTraceID(a)
	ensureTraceID(b)
	if a.TraceID == b.TraceID {
		t.Fatal("two requests received the same fallback trace id")
	}
}

// A-11: the database writer applies the same fallback, so a row never stores the
// session id in trace_id.
func TestUsageWriterDoesNotPersistSessionIDAsTraceID(t *testing.T) {
	repo := newDialectRepo(t)
	rec := &UsageLogRecord{SessionID: "sess-written", Model: "m", StatusCode: 200, TotalTokens: 3}
	if err := repo.RecordUsageLogContext(t.Context(), rec); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if rec.TraceID == rec.SessionID {
		t.Fatal("writer kept session id as trace id")
	}
	var stored string
	if err := repo.db.QueryRow(`SELECT trace_id FROM usage_logs WHERE billing_id = ?`, rec.BillingID).Scan(&stored); err != nil {
		t.Fatalf("read trace_id: %v", err)
	}
	if stored == "sess-written" {
		t.Fatal("persisted trace_id is the session id")
	}
	if stored != rec.TraceID {
		t.Fatalf("persisted trace_id = %q, want %q", stored, rec.TraceID)
	}
}

// E-11: constructing a repository registers the process-wide instance through the
// single writer, so the setter is reachable from main.
func TestNewRepositoryRegistersGlobalRepository(t *testing.T) {
	previous := GetGlobalRepository()
	t.Cleanup(func() { SetGlobalRepository(previous) })

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewRepository(db)
	if got := GetGlobalRepository(); got != repo {
		t.Fatalf("GetGlobalRepository() = %p, want the repository just constructed (%p)", got, repo)
	}
}
