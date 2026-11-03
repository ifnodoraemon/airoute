package storage

import (
	"database/sql"
	"errors"
	"testing"
)

// findChannel is the test-side replacement for the deleted Repository.GetChannel.
// The production read paths use ListChannels (there is no single-channel read), so
// the tests read through the same API the gateway does.
func findChannel(t *testing.T, repo *Repository, id int64) (*ChannelRecord, error) {
	t.Helper()
	channels, err := repo.ListChannels()
	if err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if ch.ID == id {
			return ch, nil
		}
	}
	return nil, sql.ErrNoRows
}

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

	stored, err := findChannel(t, repo, rec.ID)
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
	stale, err := findChannel(t, repo, rec.ID)
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

	stored, err := findChannel(t, repo, rec.ID)
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

	stored, err := findChannel(t, repo, rec.ID)
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

func TestDeleteChannelPrunesModelRoutes(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	c1 := seedPatchChannel(t, repo, &ChannelRecord{
		Name: "ch-1", Type: "openai", BaseURL: "http://1.invalid",
		APIKey: "k1", Models: []string{"gpt-4o"}, Priority: 1, Weight: 10, Status: "active",
	})
	c2 := seedPatchChannel(t, repo, &ChannelRecord{
		Name: "ch-2", Type: "openai", BaseURL: "http://2.invalid",
		APIKey: "k2", Models: []string{"gpt-4o"}, Priority: 1, Weight: 10, Status: "active",
	})

	// Manually insert model_route referencing both channels
	provJSON := `[{"channel_id":` + string(rune('0'+c1.ID)) + `,"channel_name":"ch-1"},{"channel_id":` + string(rune('0'+c2.ID)) + `,"channel_name":"ch-2"}]`
	_, err = db.Exec(`INSERT INTO model_routes (model, disabled, providers) VALUES (?, 0, ?)`, "gpt-4o", provJSON)
	if err != nil {
		t.Fatalf("insert model_route: %v", err)
	}

	// Delete c1
	if err := repo.DeleteChannel(c1.ID); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}

	// Verify model_routes providers updated to exclude c1
	var updatedProviders string
	err = db.QueryRow(`SELECT providers FROM model_routes WHERE model = ?`, "gpt-4o").Scan(&updatedProviders)
	if err != nil {
		t.Fatalf("query model_routes: %v", err)
	}
	if stringContains(updatedProviders, `"channel_id":`+string(rune('0'+c1.ID))) {
		t.Errorf("deleted channel %d still present in model_routes providers: %s", c1.ID, updatedProviders)
	}
	if !stringContains(updatedProviders, `"channel_id":`+string(rune('0'+c2.ID))) {
		t.Errorf("active channel %d missing in model_routes providers: %s", c2.ID, updatedProviders)
	}
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && indexOfString(s, substr) >= 0))
}

func indexOfString(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
