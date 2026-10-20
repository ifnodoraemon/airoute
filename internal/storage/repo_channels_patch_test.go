package storage

import (
	"database/sql"
	"errors"
	"testing"
)

func seedPatchChannel(t *testing.T, repo *Repository, rec *ChannelRecord) *ChannelRecord {
	t.Helper()
	if err := repo.CreateChannel(rec); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	return rec
}

func TestPatchChannelAppliesAndPersists(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	rec := seedPatchChannel(t, repo, &ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "k", Models: []string{"m1"}, Priority: 1, Weight: 10, Status: "active",
	})

	updated, err := repo.PatchChannel(rec.ID, func(cur *ChannelRecord) error {
		cur.Name = "renamed"
		cur.Models = append(cur.Models, "m2")
		return nil
	})
	if err != nil {
		t.Fatalf("PatchChannel: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("returned name = %q, want renamed", updated.Name)
	}

	stored, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if stored.Name != "renamed" {
		t.Errorf("stored name = %q, want renamed", stored.Name)
	}
	if len(stored.Models) != 2 {
		t.Errorf("stored models = %v, want two entries", stored.Models)
	}
	if stored.APIKey != "k" {
		t.Errorf("untouched field changed: api_key = %q", stored.APIKey)
	}
}

func TestPatchChannelMergesAgainstTheCurrentRow(t *testing.T) {
	// The property that fixes the lost update: the mutation is handed the row as
	// it is at patch time, not the row a caller read earlier.
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	rec := seedPatchChannel(t, repo, &ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "credential-v1", Priority: 1, Weight: 10, Status: "active",
	})

	// A second writer changes the credential through the ordinary full-row path.
	stale, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	concurrent := *stale
	concurrent.APIKey = "credential-v2"
	if err := repo.UpdateChannel(&concurrent); err != nil {
		t.Fatalf("concurrent UpdateChannel: %v", err)
	}

	// The first writer now patches only the name. It must not resurrect the
	// credential it read before the second writer ran.
	if _, err := repo.PatchChannel(rec.ID, func(cur *ChannelRecord) error {
		cur.Name = "renamed"
		return nil
	}); err != nil {
		t.Fatalf("PatchChannel: %v", err)
	}

	stored, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if stored.APIKey != "credential-v2" {
		t.Errorf("the concurrent credential change was lost: api_key = %q, want credential-v2", stored.APIKey)
	}
	if stored.Name != "renamed" {
		t.Errorf("name = %q, want renamed", stored.Name)
	}
	_ = stale
}

func TestPatchChannelReportsMissingRow(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	_, err = repo.PatchChannel(4242, func(*ChannelRecord) error {
		t.Fatal("the mutation must not run for a missing row")
		return nil
	})

	if !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("err = %v, want ErrChannelNotFound", err)
	}
	// The underlying sentinel stays matchable so callers that already test for it
	// keep working.
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v does not wrap sql.ErrNoRows", err)
	}
}

func TestPatchChannelAbortsWhenTheMutationFails(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	rec := seedPatchChannel(t, repo, &ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "k", Priority: 1, Weight: 10, Status: "active",
	})

	boom := errors.New("rejected by the mutation")
	_, err = repo.PatchChannel(rec.ID, func(cur *ChannelRecord) error {
		cur.Name = "should-not-persist"
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the mutation error", err)
	}

	stored, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if stored.Name != "chan-a" {
		t.Errorf("a rejected patch was persisted: name = %q", stored.Name)
	}
}

func TestPatchChannelRejectsNilMutation(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	if _, err := repo.PatchChannel(1, nil); err == nil {
		t.Fatal("a nil mutation function was accepted")
	}
}
