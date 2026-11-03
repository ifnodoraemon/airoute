package storage

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestResponseContextRoundTrip(t *testing.T) {
	repo := newResponseContextRepo(t)
	payload := []byte(`[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]`)

	if err := repo.SaveResponseContext(t.Context(), "resp_roundtrip", "user:7", payload); err != nil {
		t.Fatalf("save: %v", err)
	}

	stored, err := repo.LoadResponseContext(t.Context(), "resp_roundtrip", "user:7")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if stored.ID != "resp_roundtrip" || stored.Owner != "user:7" {
		t.Fatalf("stored identity = %+v", stored)
	}
	if !bytes.Equal(stored.Messages, payload) {
		t.Fatalf("messages = %s, want %s", stored.Messages, payload)
	}

	// The row must carry the ~7 day TTL, not the database default.
	var remainingHours float64
	if err := repo.db.QueryRow(
		`SELECT (julianday(expires_at) - julianday('now')) * 24 FROM response_contexts WHERE id = ?`,
		"resp_roundtrip",
	).Scan(&remainingHours); err != nil {
		t.Fatalf("query ttl: %v", err)
	}
	wantHours := ResponseContextTTL.Hours()
	if remainingHours < wantHours-1 || remainingHours > wantHours+1 {
		t.Fatalf("ttl = %.1f hours, want about %.0f", remainingHours, wantHours)
	}
}

// A response id must only be resolvable by the caller that created it: the
// conversation can contain another customer's prompts and tool output.
func TestResponseContextOwnerMismatchIsNotFound(t *testing.T) {
	repo := newResponseContextRepo(t)
	if err := repo.SaveResponseContext(t.Context(), "resp_owned", "user:7", []byte(`[{"role":"user","content":"secret"}]`)); err != nil {
		t.Fatalf("save: %v", err)
	}

	for _, owner := range []string{"user:8", "key:deadbeef", ""} {
		if _, err := repo.LoadResponseContext(t.Context(), "resp_owned", owner); !errors.Is(err, ErrResponseContextNotFound) {
			t.Fatalf("load as owner %q = %v, want ErrResponseContextNotFound", owner, err)
		}
	}
	if _, err := repo.LoadResponseContext(t.Context(), "resp_missing", "user:7"); !errors.Is(err, ErrResponseContextNotFound) {
		t.Fatalf("load of a missing id = %v, want ErrResponseContextNotFound", err)
	}
}

// Expired contexts must behave exactly like missing ones and be pruned.
func TestResponseContextExpiryAndPrune(t *testing.T) {
	repo := newResponseContextRepo(t)
	insert := func(id string, expiresAt time.Time) {
		t.Helper()
		if _, err := repo.db.Exec(`INSERT INTO response_contexts (id, owner, messages, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?)`,
			id, "user:7", `[]`,
			expiresAt.Add(-ResponseContextTTL).Format(legacySecondTimestampLayout),
			expiresAt.Format(legacySecondTimestampLayout)); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	now := time.Now().UTC()
	insert("resp_expired", now.Add(-time.Minute))
	insert("resp_fresh", now.Add(time.Hour))

	if _, err := repo.LoadResponseContext(t.Context(), "resp_expired", "user:7"); !errors.Is(err, ErrResponseContextNotFound) {
		t.Fatalf("expired context load = %v, want ErrResponseContextNotFound", err)
	}
	if _, err := repo.LoadResponseContext(t.Context(), "resp_fresh", "user:7"); err != nil {
		t.Fatalf("fresh context load: %v", err)
	}

	deleted, err := repo.PruneResponseContexts(now)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("prune deleted %d rows, want 1", deleted)
	}
	var remaining int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM response_contexts WHERE id = 'resp_fresh'`).Scan(&remaining); err != nil {
		t.Fatalf("count fresh rows: %v", err)
	}
	if remaining != 1 {
		t.Fatal("prune removed a context that had not expired yet")
	}
}

// Saving must not require the caller to have pruned first: the write path keeps
// the table bounded on its own.
func TestResponseContextSaveRejectsIncompleteRows(t *testing.T) {
	repo := newResponseContextRepo(t)
	if err := repo.SaveResponseContext(t.Context(), "", "user:7", []byte(`[]`)); err == nil {
		t.Fatal("expected an error for an empty response id")
	}
	if err := repo.SaveResponseContext(t.Context(), "resp_x", "", []byte(`[]`)); err == nil {
		t.Fatal("expected an error for an empty owner")
	}
}
