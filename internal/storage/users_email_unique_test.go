package storage

import (
	"errors"
	"sync"
	"testing"
)

func newUserTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

// Migration 27 must actually create the partial unique index; without it the
// registration check stays a check-then-insert race.
func TestUsersEmailUniqueIndexExists(t *testing.T) {
	repo := newUserTestRepo(t)

	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, usersEmailUniqueIndex).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected index %q to exist, got count %d", usersEmailUniqueIndex, count)
	}

	// Re-running the migration step must be a no-op (idempotency).
	if err := createUsersEmailUniqueIndex(repo.db); err != nil {
		t.Fatalf("index creation is not idempotent: %v", err)
	}
}

// A duplicate email must be rejected deterministically, including when only the
// case differs.
func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	repo := newUserTestRepo(t)

	first := &UserRecord{Username: "alice", Email: "alice@example.com", PasswordHash: "x"}
	if err := repo.CreateUser(first); err != nil {
		t.Fatalf("create first user: %v", err)
	}

	for _, email := range []string{"alice@example.com", "ALICE@example.com", "Alice@Example.COM"} {
		dup := &UserRecord{Username: "alice-" + email, Email: email, PasswordHash: "x"}
		err := repo.CreateUser(dup)
		if !errors.Is(err, ErrEmailAlreadyRegistered) {
			t.Fatalf("email %q: got err %v, want ErrEmailAlreadyRegistered", email, err)
		}
	}
}

// Accounts without an email address are legacy/CLI-created rows and must not be
// constrained by the partial index.
func TestCreateUserAllowsEmptyEmail(t *testing.T) {
	repo := newUserTestRepo(t)

	for _, name := range []string{"cli-a", "cli-b"} {
		if err := repo.CreateUser(&UserRecord{Username: name, PasswordHash: "x"}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
}

// The database-level guarantee (not just the pre-check) must be exercised: with
// the unique index in place, a conflicting raw insert is classified as the
// registration conflict.
func TestInsertUserClassifiesUniqueViolation(t *testing.T) {
	repo := newUserTestRepo(t)

	if err := repo.CreateUser(&UserRecord{Username: "bob", Email: "bob@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	// Bypass insertUser's pre-check by issuing the insert directly, simulating a
	// concurrent registration that passed the check before the winner committed.
	_, err := repo.db.Exec(`INSERT INTO users (username, email, password_hash, role, status, balance, balance_units, group_name, updated_at)
		VALUES (?, ?, ?, 'user', 'active', 0, 0, 'default', CURRENT_TIMESTAMP)`,
		"bob-race", "BOB@example.com", "x")
	if !isEmailConflictError(err) {
		t.Fatalf("expected the unique index to reject the duplicate, got %v", err)
	}
	if !errors.Is(conflictAsSentinel(err), ErrEmailAlreadyRegistered) {
		t.Fatal("unique violation was not mapped to ErrEmailAlreadyRegistered")
	}
}

// conflictAsSentinel mirrors insertUser's error classification for the test.
func conflictAsSentinel(err error) error {
	if isEmailConflictError(err) {
		return ErrEmailAlreadyRegistered
	}
	return err
}

// Concurrent registrations of the same email: exactly one wins, the rest get the
// conflict sentinel (never a generic error and never a second account).
func TestConcurrentRegistrationSameEmail(t *testing.T) {
	repo := newUserTestRepo(t)

	const attempts = 8
	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = repo.CreateUser(&UserRecord{
				Username:     "racer-" + string(rune('a'+idx)),
				Email:        "race@example.com",
				PasswordHash: "x",
			})
		}(i)
	}
	wg.Wait()

	created, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrEmailAlreadyRegistered):
			conflicts++
		default:
			t.Fatalf("unexpected error from concurrent registration: %v", err)
		}
	}
	if created != 1 || conflicts != attempts-1 {
		t.Fatalf("got created=%d conflicts=%d, want created=1 conflicts=%d", created, conflicts, attempts-1)
	}

	var rows int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM users WHERE LOWER(email) = 'race@example.com'`).Scan(&rows); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected exactly one account for the address, got %d", rows)
	}
}

// Registration creates the account and its initial key in one transaction; a
// duplicate email must roll the transaction back instead of leaving a user row
// without a key (or a key without a user).
func TestCreateUserWithAPIKeyRollsBackOnDuplicateEmail(t *testing.T) {
	repo := newUserTestRepo(t)

	existing := &UserRecord{Username: "carol", Email: "carol@example.com", PasswordHash: "x"}
	if err := repo.CreateUser(existing); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	err := repo.CreateUserWithAPIKey(
		&UserRecord{Username: "carol-dup", Email: "Carol@Example.com", PasswordHash: "x"},
		&APIKeyRecord{Key: "sk-carol-dup", TenantID: "carol-dup", AllowedModels: []string{"*"}, Status: "active"},
	)
	if !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("got err %v, want ErrEmailAlreadyRegistered", err)
	}

	var users int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM users WHERE LOWER(username) = 'carol-dup'`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	var keys int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE "key" = ?`, "sk-carol-dup").Scan(&keys); err != nil {
		t.Fatalf("count keys: %v", err)
	}
	if users != 0 || keys != 0 {
		t.Fatalf("failed registration left rows behind: users=%d keys=%d", users, keys)
	}
}

// Email lookup must be case-insensitive and deterministic.
func TestGetUserByEmailIsCaseInsensitive(t *testing.T) {
	repo := newUserTestRepo(t)

	u := &UserRecord{Username: "dave", Email: "dave@example.com", PasswordHash: "x"}
	if err := repo.CreateUser(u); err != nil {
		t.Fatalf("create dave: %v", err)
	}

	got, err := repo.GetUserByEmail("DAVE@example.com")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("lookup returned id %d, want %d", got.ID, u.ID)
	}
}
