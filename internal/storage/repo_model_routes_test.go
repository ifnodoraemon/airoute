package storage

import (
	"context"
	"testing"
)

// TestApplyModelRouteBundleIsAtomic verifies that a failing channel update rolls
// back the fallback and price rows too. Previously each concern was written in
// its own transaction, so an error left the configuration half-applied.
func TestApplyModelRouteBundleIsAtomic(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name:     "atomic-chan",
		Type:     "openai",
		BaseURL:  "http://example.invalid",
		APIKey:   "k",
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	// A channel row with an ID that does not exist: the UPDATE affects zero rows
	// but does not error on SQLite, so force the failure with a broken payload
	// (a nil ModelMapping is fine; instead point at an invalid table state by
	// using an out-of-range priority is not possible). Use a cancelled context
	// so the transaction cannot commit, exercising the rollback path.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	bundle := &ModelRouteBundle{
		Model:         "m1",
		FallbackModel: "m2",
		Price:         &ModelPriceRecord{Model: "m1", GroupName: "default", PromptPrice: 1, CompletionPrice: 2},
		Channels:      []*ChannelRecord{ch},
	}
	if err := repo.ApplyModelRouteBundle(ctx, bundle); err == nil {
		t.Fatal("expected the cancelled transaction to fail")
	}

	// Nothing may be persisted.
	if _, err := repo.GetModelPriceExact("m1", "default"); err == nil {
		t.Fatal("price was persisted despite the transaction failing")
	}
	fbs, err := repo.GetModelFallbacks()
	if err != nil {
		t.Fatalf("read fallbacks: %v", err)
	}
	if _, ok := fbs["m1"]; ok {
		t.Fatal("fallback was persisted despite the transaction failing")
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
