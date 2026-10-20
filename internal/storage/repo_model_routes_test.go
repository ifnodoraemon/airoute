package storage

import (
	"context"
	"testing"
)

// TestApplyModelRouteBundleIsAtomic verifies that a failure in the final step
// (channel update) rolls back the fallback and price rows written by the earlier
// steps. Previously each concern was written in its own transaction, so an error
// left the configuration half-applied.
//
// The failure must be raised *after* the earlier steps have written inside the
// transaction, otherwise the test proves nothing about rollback. It is forced by
// naming a channel that does not exist: the plan is built from a read taken
// outside the transaction, so a channel deleted in the meantime is exactly what
// that looks like, and the whole change must be abandoned.
//
// An earlier version of this test cancelled the context before
// ApplyModelRouteBundle was even called, so it only exercised "BeginTx fails"
// and never reached the partial writes it claimed to cover.
func TestApplyModelRouteBundleIsAtomic(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	// Two pre-existing channels. The bundle updates the first (a write that must
	// be rolled back) and then references a channel that is not there.
	first := &ChannelRecord{
		Name:     "atomic-chan-a",
		Type:     "openai",
		BaseURL:  "http://a.invalid",
		APIKey:   "k",
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}
	second := &ChannelRecord{
		Name:     "atomic-chan-b",
		Type:     "openai",
		BaseURL:  "http://b.invalid",
		APIKey:   "k",
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}
	for _, ch := range []*ChannelRecord{first, second} {
		if err := repo.CreateChannel(ch); err != nil {
			t.Fatalf("create channel %s: %v", ch.Name, err)
		}
	}

	bundle := &ModelRouteBundle{
		Model:         "m1",
		FallbackModel: "m2",
		Price:         &ModelPriceRecord{Model: "m1", GroupName: "default", PromptPrice: 1, CompletionPrice: 2},
		Channels: []ChannelRoutingUpdate{
			// Step 3a: a legitimate update the transaction performs before the
			// failure.
			{ID: first.ID, AddModels: []string{"m9"}, Priority: 2, Weight: 20},
			// Step 3b: a channel that does not exist.
			{ID: second.ID + 100000, AddModels: []string{"m1"}},
		},
	}
	if err := repo.ApplyModelRouteBundle(context.Background(), bundle); err == nil {
		t.Fatal("expected the vanished channel to fail the bundle")
	}

	// Step 2: the price row must not survive the rollback.
	if _, err := repo.GetModelPriceExact("m1", "default"); err == nil {
		t.Error("price was persisted despite the transaction failing: step 2 was not rolled back")
	}

	// Step 1: neither may the fallback row.
	fbs, err := repo.GetModelFallbacks()
	if err != nil {
		t.Fatalf("read fallbacks: %v", err)
	}
	if _, ok := fbs["m1"]; ok {
		t.Error("fallback was persisted despite the transaction failing: step 1 was not rolled back")
	}

	// Step 3a: the update that succeeded before the failure must be undone too.
	// The assertions cover the fields the bundle actually changes, so they would
	// fail if the first write had been committed.
	gotFirst, err := repo.GetChannel(first.ID)
	if err != nil {
		t.Fatalf("reload first channel: %v", err)
	}
	if len(gotFirst.Models) != 1 || gotFirst.Models[0] != "m1" {
		t.Errorf("first channel model list was not rolled back: %v", gotFirst.Models)
	}
	if gotFirst.Priority != 1 || gotFirst.Weight != 10 {
		t.Errorf("first channel routing fields were not rolled back: priority=%d weight=%d",
			gotFirst.Priority, gotFirst.Weight)
	}

	// Fields the bundle does not touch must be untouched as well.
	if gotFirst.Name != "atomic-chan-a" || gotFirst.BaseURL != "http://a.invalid" || gotFirst.APIKey != "k" {
		t.Errorf("first channel lost fields the route change does not own: name=%q base_url=%q api_key=%q",
			gotFirst.Name, gotFirst.BaseURL, gotFirst.APIKey)
	}

	// The channel that was never part of the plan must be untouched.
	gotSecond, err := repo.GetChannel(second.ID)
	if err != nil {
		t.Fatalf("reload second channel: %v", err)
	}
	if gotSecond.Name != "atomic-chan-b" {
		t.Errorf("second channel name changed despite the failed update: %q", gotSecond.Name)
	}
}

// TestApplyModelRouteBundleWritesGroupScopedPrice pins that the price row lands
// in the requested group and does not leak into the default group.
func TestApplyModelRouteBundleWritesGroupScopedPrice(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	if err := repo.SaveModelPrice(&ModelPriceRecord{
		Model: "gpt-x", GroupName: "default", PromptPrice: 10, CompletionPrice: 20, Currency: "CNY",
	}); err != nil {
		t.Fatalf("seed default price: %v", err)
	}

	bundle := &ModelRouteBundle{
		Model: "gpt-x",
		Price: &ModelPriceRecord{Model: "gpt-x", GroupName: "vip", PromptPrice: 5, CompletionPrice: 8, Currency: "CNY"},
	}
	if err := repo.ApplyModelRouteBundle(context.Background(), bundle); err != nil {
		t.Fatalf("apply bundle: %v", err)
	}

	vip, err := repo.GetModelPriceExact("gpt-x", "vip")
	if err != nil {
		t.Fatalf("vip price missing: %v", err)
	}
	if vip.PromptPrice != 5 || vip.CompletionPrice != 8 {
		t.Fatalf("vip price wrong: %+v", vip)
	}

	def, err := repo.GetModelPriceExact("gpt-x", "default")
	if err != nil {
		t.Fatalf("default price missing: %v", err)
	}
	if def.PromptPrice != 10 || def.CompletionPrice != 20 {
		t.Fatalf("default group was overwritten by a group-scoped save: %+v", def)
	}
}

// TestApplyModelRouteBundleKeepsConcurrentChannelEdits covers the same hazard the
// channel service had: the route editor builds its channel rows from a read taken
// outside the transaction, so a whole-row write would revert anything a second
// administrator changed in between. The route change owns the routing fields; it
// must not have an opinion about credentials, endpoints or timeouts.
func TestApplyModelRouteBundleKeepsConcurrentChannelEdits(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name: "route-chan", Type: "openai", BaseURL: "http://original.invalid",
		APIKey: "credential-v1", Models: []string{"other"}, Priority: 1, Weight: 5,
		TimeoutSeconds: 60, Protocols: []string{"openai_chat"}, Status: "active",
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	// The route editor reads the channel to compute its change set...
	stale, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	routeRow := *stale
	routeRow.Models = []string{"other", "gpt-x"}
	routeRow.Priority = 3
	routeRow.Weight = 20

	// ...and a second administrator changes the credential and endpoint before
	// the route save commits.
	concurrent := *stale
	concurrent.APIKey = "credential-v2"
	concurrent.BaseURL = "http://updated.invalid"
	concurrent.TimeoutSeconds = 120
	if err := repo.UpdateChannel(&concurrent); err != nil {
		t.Fatalf("concurrent UpdateChannel: %v", err)
	}

	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model: "gpt-x",
		Channels: []ChannelRoutingUpdate{{
			ID:        routeRow.ID,
			AddModels: []string{"gpt-x"},
			Priority:  3,
			Weight:    20,
		}},
	}); err != nil {
		t.Fatalf("ApplyModelRouteBundle: %v", err)
	}

	stored, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}

	// The route change applied...
	if stored.Priority != 3 || stored.Weight != 20 {
		t.Errorf("routing fields not applied: priority=%d weight=%d", stored.Priority, stored.Weight)
	}
	found := false
	for _, m := range stored.Models {
		if m == "gpt-x" {
			found = true
		}
	}
	if !found {
		t.Errorf("model was not added to the channel: %v", stored.Models)
	}

	// ...and the concurrent edits survived.
	if stored.APIKey != "credential-v2" {
		t.Errorf("the concurrent credential change was reverted: api_key = %q", stored.APIKey)
	}
	if stored.BaseURL != "http://updated.invalid" {
		t.Errorf("the concurrent endpoint change was reverted: base_url = %q", stored.BaseURL)
	}
	if stored.TimeoutSeconds != 120 {
		t.Errorf("the concurrent timeout change was reverted: timeout = %d", stored.TimeoutSeconds)
	}
	if len(stored.Protocols) != 1 || stored.Protocols[0] != "openai_chat" {
		t.Errorf("protocols were disturbed: %v", stored.Protocols)
	}
}

// TestApplyModelRouteBundleKeepsAConcurrentRename closes the last field the route
// change used to write from its own earlier read. The route editor has no opinion
// about a channel's name, so a rename made by a second administrator between the
// plan being built and the transaction committing must survive.
func TestApplyModelRouteBundleKeepsAConcurrentRename(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name: "original-name", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "k", Models: []string{"other"}, Priority: 1, Weight: 5, Status: "active",
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	// The route editor reads the channel to build its plan...
	stale, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	routeRow := *stale
	routeRow.Models = []string{"other", "gpt-x"}

	// ...and a second administrator renames the channel before the save commits.
	renamed := *stale
	renamed.Name = "renamed-by-someone-else"
	if err := repo.UpdateChannel(&renamed); err != nil {
		t.Fatalf("concurrent rename: %v", err)
	}

	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:    "gpt-x",
		Channels: []ChannelRoutingUpdate{{ID: routeRow.ID, AddModels: []string{"gpt-x"}}},
	}); err != nil {
		t.Fatalf("ApplyModelRouteBundle: %v", err)
	}

	stored, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if stored.Name != "renamed-by-someone-else" {
		t.Errorf("the concurrent rename was reverted: name = %q", stored.Name)
	}
	found := false
	for _, m := range stored.Models {
		if m == "gpt-x" {
			found = true
		}
	}
	if !found {
		t.Errorf("the route change was not applied: models = %v", stored.Models)
	}
}

// TestApplyModelRouteBundleKeepsConcurrentModelAdds is the regression this whole
// delta shape exists for.
//
// Two administrators add different models to the same shared channel. Both build
// their change set from the same read, so both see the channel serving only
// "shared-base". With a replacement list the second save writes the list it read
// and the first administrator's model silently disappears. With a delta applied
// inside the transaction, both additions survive.
func TestApplyModelRouteBundleKeepsConcurrentModelAdds(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name: "shared-channel", Type: "openai", BaseURL: "http://shared.invalid",
		APIKey: "k", Models: []string{"shared-base"}, Priority: 1, Weight: 10, Status: "active",
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	// Both administrators read the channel and plan from that same snapshot.
	snapshot, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if len(snapshot.Models) != 1 || snapshot.Models[0] != "shared-base" {
		t.Fatalf("unexpected starting state: %v", snapshot.Models)
	}

	// Administrator A adds model-a and commits first.
	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:    "model-a",
		Channels: []ChannelRoutingUpdate{{ID: ch.ID, AddModels: []string{"model-a"}}},
	}); err != nil {
		t.Fatalf("apply A: %v", err)
	}

	// Administrator B adds model-b from the stale snapshot and commits second.
	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:    "model-b",
		Channels: []ChannelRoutingUpdate{{ID: ch.ID, AddModels: []string{"model-b"}}},
	}); err != nil {
		t.Fatalf("apply B: %v", err)
	}

	stored, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}

	// All three models must be present. Before the delta shape this was
	// [shared-base, model-b]: A's model had been overwritten.
	got := map[string]bool{}
	for _, m := range stored.Models {
		got[m] = true
	}
	for _, want := range []string{"shared-base", "model-a", "model-b"} {
		if !got[want] {
			t.Errorf("model %q was lost; channel serves %v", want, stored.Models)
		}
	}
}

// TestApplyModelRouteBundleKeepsConcurrentMappingAdds covers the same race for the
// model mapping table: two administrators map different models to different
// upstreams, and neither entry may be dropped.
func TestApplyModelRouteBundleKeepsConcurrentMappingAdds(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name: "mapping-channel", Type: "openai", BaseURL: "http://m.invalid",
		APIKey: "k", Models: []string{"shared-base"}, Priority: 1, Weight: 10, Status: "active",
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model: "model-a",
		Channels: []ChannelRoutingUpdate{{
			ID: ch.ID, AddModels: []string{"model-a"},
			SetMapping: map[string]string{"model-a": "upstream-a"},
		}},
	}); err != nil {
		t.Fatalf("apply A: %v", err)
	}

	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model: "model-b",
		Channels: []ChannelRoutingUpdate{{
			ID: ch.ID, AddModels: []string{"model-b"},
			SetMapping: map[string]string{"model-b": "upstream-b"},
		}},
	}); err != nil {
		t.Fatalf("apply B: %v", err)
	}

	stored, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if stored.ModelMapping["model-a"] != "upstream-a" {
		t.Errorf("mapping for model-a was lost: %v", stored.ModelMapping)
	}
	if stored.ModelMapping["model-b"] != "upstream-b" {
		t.Errorf("mapping for model-b was lost: %v", stored.ModelMapping)
	}
}

// TestApplyModelRouteBundleSkipsNoOpWrites pins the write-avoidance: a delta that
// changes nothing must not touch the row, so an unchanged channel is not rewritten
// on every route save.
func TestApplyModelRouteBundleSkipsNoOpWrites(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name: "noop-channel", Type: "openai", BaseURL: "http://noop.invalid",
		APIKey: "k", Models: []string{"gpt-x"}, Priority: 1, Weight: 10, Status: "active",
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	before, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}

	// gpt-x is already served and both scalars are unset, so this is a no-op.
	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:    "gpt-x",
		Channels: []ChannelRoutingUpdate{{ID: ch.ID, AddModels: []string{"gpt-x"}}},
	}); err != nil {
		t.Fatalf("ApplyModelRouteBundle: %v", err)
	}

	after, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("a no-op delta rewrote the row: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}
}
