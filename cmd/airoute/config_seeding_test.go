package main

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// C-02 regression suite: the boot-time seeder must never duplicate the YAML
// configuration, which it used to do by treating a failed ListChannels/ListAPIKeys
// read as "the database is empty".

func newSeedingRepo(t *testing.T) (*storage.Repository, *storage.DB) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return storage.NewRepository(db), db
}

func seedingConfig() *config.Config {
	return &config.Config{
		Channels: []model.ChannelConfig{
			{Name: "primary", Type: "openai", BaseURL: "https://a.invalid", Models: []string{"gpt-4o"}},
			{Name: "primary", Type: "openai", BaseURL: "https://duplicate.invalid", Models: []string{"gpt-4o"}},
			{Name: "secondary", Type: "openai", BaseURL: "https://b.invalid", Models: []string{"gpt-4o"}},
		},
		APIKeys: []model.APIKeyConfig{
			{Key: "sk-seed-one", TenantID: "admin"},
			{Key: "sk-seed-one", TenantID: "admin"},
		},
	}
}

func TestSeedFromConfigIsIdempotent(t *testing.T) {
	repo, _ := newSeedingRepo(t)
	cfg := seedingConfig()

	if err := seedChannelsFromConfig(repo, cfg); err != nil {
		t.Fatalf("first channel seed: %v", err)
	}
	if err := seedChannelsFromConfig(repo, cfg); err != nil {
		t.Fatalf("second channel seed: %v", err)
	}
	channels, err := repo.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("channels after two seeds = %d, want 2 (duplicate name must be collapsed, re-seed must be a no-op)", len(channels))
	}

	if err := seedAPIKeysFromConfig(repo, cfg); err != nil {
		t.Fatalf("first key seed: %v", err)
	}
	if err := seedAPIKeysFromConfig(repo, cfg); err != nil {
		t.Fatalf("second key seed: %v", err)
	}
	keys, err := repo.ListAPIKeys()
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("api keys after two seeds = %d, want 1 (duplicate key collapsed, re-seed no-op)", len(keys))
	}
}

// TestSeedSkipsWhenTheReadFails is the original defect: a database error at boot
// must not be read as "empty database" and followed by a full re-seed.
func TestSeedSkipsWhenTheReadFails(t *testing.T) {
	repo, db := newSeedingRepo(t)
	cfg := seedingConfig()

	if err := seedChannelsFromConfig(repo, cfg); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	channels, err := repo.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("initial channels = %d, want 2", len(channels))
	}

	// Make the repository unavailable, the way a failed database behaves at boot,
	// and try to seed again: the read error must abort the seeding instead of being
	// mistaken for "no rows".
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if err := seedChannelsFromConfig(repo, cfg); err == nil {
		t.Fatal("seeding succeeded on a failed read; a database error is being treated as an empty database")
	}
	if err := seedAPIKeysFromConfig(repo, cfg); err == nil {
		t.Fatal("key seeding succeeded on a failed read")
	}
}

// TestSeedNoOpsWithoutConfig keeps the no-configuration path free of log noise and
// database writes.
func TestSeedNoOpsWithoutConfig(t *testing.T) {
	repo, _ := newSeedingRepo(t)
	if err := seedChannelsFromConfig(repo, &config.Config{}); err != nil {
		t.Fatalf("empty config: %v", err)
	}
	if err := seedAPIKeysFromConfig(nil, seedingConfig()); err != nil {
		t.Fatalf("nil repository: %v", err)
	}
}
