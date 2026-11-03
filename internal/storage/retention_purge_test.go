package storage

import (
	"fmt"
	"testing"
	"time"
)

// Every retention purge now runs through one bounded-batch loop
// (purgeBeforeCutoff). These tests pin the three behaviours that loop exists
// for — draining a table across several batches, stopping on a short batch, and
// returning the partial count together with the error — so a later change to the
// shared helper cannot silently regress any purge built on it.
func TestPurgeBeforeCutoffBatchingAndFailure(t *testing.T) {
	cutoff := time.Now().UTC()

	t.Run("drains in bounded batches", func(t *testing.T) {
		repo := newRetentionRepo(t)
		want := retentionPruneBatchSize + 7
		insertExpiredVerificationCodes(t, repo, want, cutoff)

		deleted, err := repo.PurgeVerificationCodesBefore(cutoff)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if int64(want) != deleted {
			t.Fatalf("purge deleted %d rows, want %d", deleted, want)
		}
		var remaining int
		if err := repo.db.QueryRow(`SELECT COUNT(*) FROM verification_codes`).Scan(&remaining); err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if remaining != 0 {
			t.Fatalf("%d expired rows survived the purge", remaining)
		}
	})

	t.Run("stops after a short first batch", func(t *testing.T) {
		repo := newRetentionRepo(t)
		insertExpiredVerificationCodes(t, repo, 3, cutoff)

		deleted, err := repo.PurgeVerificationCodesBefore(cutoff)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if deleted != 3 {
			t.Fatalf("purge deleted %d rows, want 3", deleted)
		}
		// A second pass over a drained table must terminate immediately.
		again, err := repo.PurgeVerificationCodesBefore(cutoff)
		if err != nil {
			t.Fatalf("purge of a drained table: %v", err)
		}
		if again != 0 {
			t.Fatalf("purge of a drained table deleted %d rows, want 0", again)
		}
	})

	t.Run("reports a statement failure without a partial count", func(t *testing.T) {
		repo := newRetentionRepo(t)
		if err := repo.db.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}
		deleted, err := repo.PurgeVideoTasksBefore(cutoff)
		if err == nil {
			t.Fatal("purge against a closed database returned no error")
		}
		if deleted != 0 {
			t.Fatalf("deleted = %d after a failed statement, want 0", deleted)
		}
	})
}

// insertExpiredVerificationCodes inserts n codes whose expires_at is one hour
// before expiresAt, so a purge with that cutoff matches all of them.
func insertExpiredVerificationCodes(t *testing.T, repo *Repository, n int, expiresAt time.Time) {
	t.Helper()
	stamp := expiresAt.Add(-time.Hour).Format(legacySecondTimestampLayout)
	for i := 0; i < n; i++ {
		if _, err := repo.db.Exec(
			`INSERT INTO verification_codes (email, code, purpose, expires_at, used) VALUES (?, ?, ?, ?, 0)`,
			fmt.Sprintf("batch-%d@example.com", i), "111111", "register", stamp); err != nil {
			t.Fatalf("insert verification code %d: %v", i, err)
		}
	}
}
