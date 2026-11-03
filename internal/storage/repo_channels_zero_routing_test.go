package storage

import (
	"testing"
)

// seedRoutingChannel creates a channel whose routing fields are all non-zero, so a
// dropped explicit zero is visible in the stored row.
func seedRoutingChannel(t *testing.T, repo *Repository) *ChannelRecord {
	t.Helper()
	rec := &ChannelRecord{
		Name: "routing-chan", Type: "openai", BaseURL: "http://routing.invalid",
		Priority: 3, Weight: 7, TimeoutSeconds: 120, Status: "active",
	}
	if err := repo.CreateChannel(rec); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return rec
}

func storedRoutingFields(t *testing.T, repo *Repository, id int64) (priority, weight, timeout int) {
	t.Helper()
	stored, err := findChannel(t, repo, id)
	if err != nil {
		t.Fatalf("read channel %d: %v", id, err)
	}
	return stored.Priority, stored.Weight, stored.TimeoutSeconds
}

// D-14 regression: an update must store the routing fields exactly as given.
//
// Priority 0, weight 0 and timeout 0 are meaningful (highest priority, excluded
// from weighted selection, no per-channel deadline). The update path used to
// rewrite them to 1/10/60, so the control plane accepted an explicit zero and the
// database silently reverted it.
func TestUpdateChannelPersistsExplicitZeroRoutingFields(t *testing.T) {
	repo := newDialectRepo(t)
	rec := seedRoutingChannel(t, repo)

	rec.Priority, rec.Weight, rec.TimeoutSeconds = 0, 0, 0
	if err := repo.UpdateChannel(rec); err != nil {
		t.Fatalf("update channel: %v", err)
	}

	priority, weight, timeout := storedRoutingFields(t, repo, rec.ID)
	if priority != 0 || weight != 0 || timeout != 0 {
		t.Fatalf("explicit zeros were coerced on update: priority=%d weight=%d timeout=%d, want 0/0/0",
			priority, weight, timeout)
	}
}

// The console's update path is PatchChannel; it must apply the same semantics.
func TestPatchChannelPersistsExplicitZeroRoutingFields(t *testing.T) {
	repo := newDialectRepo(t)
	rec := seedRoutingChannel(t, repo)

	patched, err := repo.PatchChannel(rec.ID, func(stored *ChannelRecord) error {
		stored.Priority = 0
		stored.Weight = 0
		stored.TimeoutSeconds = 0
		return nil
	})
	if err != nil {
		t.Fatalf("patch channel: %v", err)
	}
	if patched.Priority != 0 || patched.Weight != 0 || patched.TimeoutSeconds != 0 {
		t.Fatalf("PatchChannel returned coerced zeros: %+v", patched)
	}

	priority, weight, timeout := storedRoutingFields(t, repo, rec.ID)
	if priority != 0 || weight != 0 || timeout != 0 {
		t.Fatalf("explicit zeros were coerced by PatchChannel: priority=%d weight=%d timeout=%d, want 0/0/0",
			priority, weight, timeout)
	}
}

// The create path keeps its own semantics: a zero there means "use the default",
// so the stored row still starts from 1/10/60. This asymmetry with update is
// deliberate and documented on updateChannelTx.
func TestCreateChannelStillDefaultsZeroRoutingFields(t *testing.T) {
	repo := newDialectRepo(t)
	rec := &ChannelRecord{
		Name: "default-routing-chan", Type: "openai", BaseURL: "http://defaults.invalid",
		Status: "active",
	}
	if err := repo.CreateChannel(rec); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if rec.Priority != 1 || rec.Weight != 10 || rec.TimeoutSeconds != 60 {
		t.Fatalf("create normalisation changed: priority=%d weight=%d timeout=%d, want 1/10/60",
			rec.Priority, rec.Weight, rec.TimeoutSeconds)
	}
	priority, weight, timeout := storedRoutingFields(t, repo, rec.ID)
	if priority != 1 || weight != 10 || timeout != 60 {
		t.Fatalf("stored create defaults = %d/%d/%d, want 1/10/60", priority, weight, timeout)
	}
}
