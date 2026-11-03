package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"
)

const migrationLockWait = 2 * time.Minute

// Migrations need to wait for a peer's complete startup, while request-path
// connections deliberately use a 500ms lock_timeout. Polling a nonblocking
// advisory lock preserves those request settings and gives startup its own bound.
func (db *DB) acquirePostgresMigrationLock(ctx context.Context) (*sql.Conn, error) {
	conn, err := db.DB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire migration connection: %w", err)
	}
	acquired := false
	defer func() {
		if !acquired {
			// A canceled query can have acquired the session lock before its result
			// reached us. Discard the physical session rather than returning a
			// potentially locked connection to the request pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			_ = conn.Close()
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext('airoute_migrations'))`).Scan(&acquired); err != nil {
			return nil, fmt.Errorf("acquire migration advisory lock: %w", err)
		}
		if acquired {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for migration advisory lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func releasePostgresMigrationLock(conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext('airoute_migrations'))`); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	_ = conn.Close()
}
