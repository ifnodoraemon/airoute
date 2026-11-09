package storage

import (
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// defaultSQLiteCheckpointInterval is how often StartSQLiteCheckpointer truncates
// the WAL when the caller asks for the default cadence.
const defaultSQLiteCheckpointInterval = 2 * time.Minute

// StartSQLiteCheckpointer starts the low-frequency WAL checkpoint that replaces
// SQLite's per-connection auto-checkpoint (disabled via wal_autocheckpoint(0)).
//
// The auto-checkpoint used to run inside whichever write transaction happened to
// push the WAL past its page threshold, charging an fsync to an unlucky settle
// call. A background ticker keeps the WAL bounded without putting that work on
// the settlement critical path. The returned stop function is idempotent and the
// caller (server.go) should invoke it on shutdown.
//
// It is a no-op for PostgreSQL and for a zero-value DB (tests), so callers do not
// need to branch on the dialect.
func (db *DB) StartSQLiteCheckpointer(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = defaultSQLiteCheckpointInterval
	}
	if db == nil || db.DB == nil || db.Dialect() != "sqlite" {
		return func() {}
	}

	done := make(chan struct{})
	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() { close(done) })
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// TRUNCATE resets the WAL to zero bytes; it blocks until readers
				// release their marks, which is acceptable at this cadence.
				if _, err := db.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
					telemetry.WarnLogger(telemetry.ScopedLogger("system"), "sqlite-wal-checkpoint-failed",
						time.Minute, "background WAL checkpoint failed; the WAL will keep growing until it succeeds",
						"error", err.Error())
				}
			}
		}
	}()
	return stop
}
