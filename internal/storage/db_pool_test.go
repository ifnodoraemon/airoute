package storage

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// openPoolFileDB opens a file-backed SQLite database. File-backed is essential:
// ":memory:" has no second-connection semantics and WAL/autocheckpoint only exist
// for real files.
func openPoolFileDB(t *testing.T, name string) *DB {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("open sqlite %s: %v", name, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// The single-connection write pool serialised every read behind the settlement
// writer. SQLite WAL allows concurrent readers, so the hot read-only queries use a
// separate pool; this asserts that pool exists and actually works concurrently.
func TestSQLiteReaderUsesSeparatePool(t *testing.T) {
	db := openPoolFileDB(t, "reader-pool.db")
	r := NewRepository(db)

	if db.Reader() == nil {
		t.Fatal("Reader() returned nil")
	}
	if db.Reader() == db.DB {
		t.Fatal("Reader() returned the single write pool; reads will queue behind writes")
	}

	u := walletTestUser(t, r, "reader-pool-user", "")

	const workers, readsPerWorker = 8, 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerWorker; i++ {
				if _, err := r.GetUserByIDContext(context.Background(), u.ID); err != nil {
					errCh <- err
					return
				}
				if _, err := r.ListModelPrices(); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent read on the read pool failed: %v", err)
	}
}

// The point of the read pool: a read must not wait for an open write transaction,
// which is exactly what happened when every query shared the single connection.
func TestSQLiteReaderNotBlockedByOpenWriteTransaction(t *testing.T) {
	db := openPoolFileDB(t, "reader-vs-writer.db")
	r := NewRepository(db)
	u := walletTestUser(t, r, "reader-vs-writer-user", "")

	tx, err := db.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, u.ID); err != nil {
		t.Fatalf("write inside transaction: %v", err)
	}

	// While the writer holds its connection and WAL write lock, the read must still
	// be served by the read pool. If it were queued on the write pool it would sit
	// here until the context deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loaded, err := r.GetUserByIDContext(ctx, u.ID)
	if err != nil {
		t.Fatalf("read blocked behind the open writer transaction: %v", err)
	}
	if loaded.ID != u.ID {
		t.Fatalf("loaded user %d, want %d", loaded.ID, u.ID)
	}
}

// An in-memory database cannot be shared by a second connection (it would be a
// different, empty database), so Reader() must fall back to the main pool there.
func TestSQLiteReaderFallsBackForMemoryDB(t *testing.T) {
	memDB, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = memDB.Close() })
	if memDB.Reader() != memDB.DB {
		t.Fatal("in-memory database must use the main pool for reads")
	}
}

// Reader() and the checkpointer must tolerate zero-value DBs: a test that builds
// &DB{} manually must not panic.
func TestReaderAndCheckpointerNilSafety(t *testing.T) {
	var nilDB *DB
	if nilDB.Reader() != nil {
		t.Fatal("nil *DB Reader() should be nil")
	}
	stop := nilDB.StartSQLiteCheckpointer(time.Second)
	stop()
	stop()

	zero := &DB{}
	if zero.Reader() != nil {
		t.Fatal("zero-value DB Reader() should be nil")
	}
	zeroStop := zero.StartSQLiteCheckpointer(0)
	zeroStop()
	zeroStop()
}

// wal_autocheckpoint(0) moves the checkpoint off the writer's critical path; a new
// connection inherits it from the DSN, so this checks the DSN rather than a lucky
// connection.
func TestSQLiteWALAutocheckpointDisabled(t *testing.T) {
	db := openPoolFileDB(t, "autocheckpoint.db")

	var autocheckpoint int
	if err := db.QueryRow(`PRAGMA wal_autocheckpoint`).Scan(&autocheckpoint); err != nil {
		t.Fatalf("read wal_autocheckpoint: %v", err)
	}
	if autocheckpoint != 0 {
		t.Fatalf("wal_autocheckpoint = %d, want 0 (background checkpointer owns it)", autocheckpoint)
	}

	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

// The background checkpointer must be a clean start/stop pair: stopping twice is
// harmless and the database keeps accepting work afterwards.
func TestStartSQLiteCheckpointerStopsIdempotently(t *testing.T) {
	db := openPoolFileDB(t, "checkpointer.db")
	r := NewRepository(db)
	walletTestUser(t, r, "checkpointer-user", "")

	stop := db.StartSQLiteCheckpointer(10 * time.Millisecond)
	time.Sleep(50 * time.Millisecond) // let at least one tick run
	stop()
	stop() // second call must not panic or double-close a channel

	// The database is still usable after the checkpointer stopped.
	if err := r.CreateUser(&UserRecord{Username: "checkpointer-user-2", PasswordHash: "synthetic", Balance: 1}); err != nil {
		t.Fatalf("write after stop: %v", err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("explicit checkpoint after stop: %v", err)
	}
}

// Close must shut both pools down: leaving the read pool open keeps WAL read marks
// and file descriptors alive after a deliberate shutdown.
func TestCloseClosesReadPool(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "close-pools.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	reader := db.Reader()
	if reader == db.DB {
		t.Fatal("test needs a separate read pool")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := reader.Ping(); err == nil {
		t.Fatal("read pool still accepting queries after Close")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second close should be harmless: %v", err)
	}
}

// The settlement retry depends on lock_timeout firing in 500ms instead of waiting
// out the 30s statement_timeout. Both the URL and keyword/value DSN forms must
// carry it, and an operator-provided `options` must still win untouched.
func TestApplyPostgresStatementTimeoutsIncludesLockTimeout(t *testing.T) {
	const wantLock = "-c lock_timeout=500"

	urlDSN := ApplyPostgresStatementTimeouts("postgres://user:pass@localhost:5432/airoute?sslmode=disable")
	rewritten, err := url.Parse(urlDSN)
	if err != nil {
		t.Fatalf("parse rewritten URL DSN: %v", err)
	}
	parsed := rewritten.Query().Get("options")
	if !strings.Contains(parsed, wantLock) {
		t.Fatalf("URL DSN options = %q, want it to contain %q", parsed, wantLock)
	}
	if !strings.Contains(parsed, "statement_timeout=30000") {
		t.Fatalf("URL DSN options = %q, want the statement timeout preserved", parsed)
	}

	keywordDSN := ApplyPostgresStatementTimeouts("host=localhost dbname=airoute sslmode=disable")
	for _, want := range []string{wantLock, "statement_timeout=30000", "idle_in_transaction_session_timeout=30000"} {
		if !strings.Contains(keywordDSN, want) {
			t.Fatalf("keyword DSN = %q, want it to contain %q", keywordDSN, want)
		}
	}

	operator := "postgres://user:pass@localhost:5432/airoute?options=-c%20lock_timeout%3D9000"
	if got := ApplyPostgresStatementTimeouts(operator); got != operator {
		t.Fatalf("operator options were overwritten: %q, want %q", got, operator)
	}
	if got := ApplyPostgresStatementTimeouts("host=localhost options='-c lock_timeout=9000'"); !strings.Contains(got, "lock_timeout=9000") {
		t.Fatalf("operator keyword options were overwritten: %q", got)
	}
}
