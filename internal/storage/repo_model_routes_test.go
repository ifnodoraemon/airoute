package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newIndependentRouteRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

func seedRouteChannel(t *testing.T, repo *Repository, name string) *ChannelRecord {
	t.Helper()
	ch := &ChannelRecord{Name: name, Type: "openai", BaseURL: "http://upstream.invalid",
		APIKey: "test-key", Models: []string{"model-a", "model-b", "*"},
		ModelMapping: map[string]string{"catalog-alias": "model-a"},
		Priority:     1, Weight: 10, Status: "active"}
	require.NoError(t, repo.CreateChannel(ch))
	return ch
}

func TestApplyModelRouteBundleIsAtomic(t *testing.T) {
	repo := newIndependentRouteRepo(t)
	ch := seedRouteChannel(t, repo, "atomic")
	before, err := repo.GetChannel(ch.ID)
	require.NoError(t, err)
	err = repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model: "model-a", FallbackModel: "model-b",
		Price:     &ModelPriceRecord{Model: "model-a", PromptPrice: 5},
		Providers: []ModelRouteProvider{{ChannelID: ch.ID, Weight: 20}, {ChannelID: ch.ID + 100000}},
	})
	require.Error(t, err)
	_, err = repo.GetModelPrice("model-a")
	require.Error(t, err)
	rules, err := repo.ListModelRouteRules()
	require.NoError(t, err)
	require.Empty(t, rules)
	fallbacks, err := repo.GetModelFallbacks()
	require.NoError(t, err)
	require.Empty(t, fallbacks)
	after, err := repo.GetChannel(ch.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

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

// Shared conformance scenario also runs against PostgreSQL in the integration suite.
func verifyIndependentRoutePolicies(t *testing.T, repo *Repository, suffix string) {
	t.Helper()
	ctx := context.Background()
	ch := seedRouteChannel(t, repo, "independent-"+suffix)
	before, err := repo.GetChannel(ch.ID)
	require.NoError(t, err)
	for _, name := range []string{"public-a-" + suffix, "public-b-" + suffix} {
		require.NoError(t, repo.ApplyModelRouteBundle(ctx, &ModelRouteBundle{
			Model: name, Providers: []ModelRouteProvider{{ChannelID: ch.ID, Priority: 3, Weight: 27, MappedModel: "model-a"}},
		}))
	}
	after, err := repo.GetChannel(ch.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "route saves must never rewrite channel records")
	channels, err := repo.ToModelChannels()
	require.NoError(t, err)
	for _, runtime := range channels {
		if runtime.ID != ch.ID {
			continue
		}
		require.True(t, runtime.SupportsModel("public-a-"+suffix))
		require.True(t, runtime.SupportsModel("public-b-"+suffix))
		require.Equal(t, "model-a", runtime.GetUpstreamModel("public-a-"+suffix))
		require.Equal(t, 3, runtime.ForModel("public-a-"+suffix).Priority)
		require.Equal(t, 27, runtime.ForModel("public-a-"+suffix).Weight)
		require.Equal(t, 10, runtime.ForModel("model-b").Weight)
	}
	require.NoError(t, repo.DeleteModelRoutes(ctx, []string{"public-a-" + suffix, "model-a"}))
	channels, err = repo.ToModelChannels()
	require.NoError(t, err)
	for _, runtime := range channels {
		require.False(t, runtime.SupportsModel("public-a-"+suffix), "wildcards must not bypass a deleted route")
		require.False(t, runtime.SupportsModel("model-a"))
	}
	after, err = repo.GetChannel(ch.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "deletions must retain channel capabilities, mappings and defaults")
	// Saving the channel again must not resurrect explicitly deleted routes.
	after.Models = append(after.Models, "new-capability")
	require.NoError(t, repo.UpdateChannel(after))
	channels, err = repo.ToModelChannels()
	require.NoError(t, err)
	for _, runtime := range channels {
		require.False(t, runtime.SupportsModel("model-a"))
	}
	// Explicit route creation restores the model without editing the catalog.
	require.NoError(t, repo.ApplyModelRouteBundle(ctx, &ModelRouteBundle{
		Model: "model-a", Providers: []ModelRouteProvider{{ChannelID: ch.ID, Priority: 1, Weight: 8, MappedModel: "model-b"}},
	}))
	channels, err = repo.ToModelChannels()
	require.NoError(t, err)
	for _, runtime := range channels {
		if runtime.ID == ch.ID {
			require.True(t, runtime.SupportsModel("model-a"))
			require.Equal(t, "model-b", runtime.GetUpstreamModel("model-a"))
		}
	}
}

func TestIndependentRoutePolicies(t *testing.T) {
	verifyIndependentRoutePolicies(t, newIndependentRouteRepo(t), "sqlite")
}

func TestRouteDeletionSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.db")
	db, err := OpenDB(path)
	require.NoError(t, err)
	repo := NewRepository(db)
	seedRouteChannel(t, repo, "persistent")
	require.NoError(t, repo.DeleteModelRoutes(context.Background(), []string{"model-a"}))
	require.NoError(t, db.Close())
	db, err = OpenDB(path)
	require.NoError(t, err)
	defer db.Close()
	channels, err := NewRepository(db).ToModelChannels()
	require.NoError(t, err)
	require.False(t, channels[0].SupportsModel("model-a"))
	require.True(t, channels[0].SupportsModel("model-b"))
}

func TestRoutePricingUpdatePreservesBindings(t *testing.T) {
	repo := newIndependentRouteRepo(t)
	ch := seedRouteChannel(t, repo, "pricing")
	ctx := context.Background()
	require.NoError(t, repo.ApplyModelRouteBundle(ctx, &ModelRouteBundle{
		Model: "alias", Providers: []ModelRouteProvider{{ChannelID: ch.ID, Priority: 2, Weight: 17, MappedModel: "model-a"}},
	}))
	require.NoError(t, repo.ApplyModelRouteBundle(ctx, &ModelRouteBundle{
		Model: "alias", Price: &ModelPriceRecord{Model: "alias", PromptPrice: 3},
	}))
	rules, err := repo.ListModelRouteRules()
	require.NoError(t, err)
	require.Len(t, rules["alias"].Providers, 1)
	require.Equal(t, 17, rules["alias"].Providers[0].Weight)
	require.NoError(t, repo.ApplyModelRouteBundle(ctx, &ModelRouteBundle{Model: "alias", Providers: []ModelRouteProvider{}}))
	channels, err := repo.ToModelChannels()
	require.NoError(t, err)
	require.False(t, channels[0].SupportsModel("alias"), "an empty provider selection must not inherit wildcard capabilities")
}

func TestBatchRouteDeletionRollsBackOnFailure(t *testing.T) {
	repo := newIndependentRouteRepo(t)
	require.NoError(t, repo.SetModelFallback("model-a", "model-b", true))
	_, err := repo.db.Exec(`CREATE TRIGGER reject_route_delete BEFORE INSERT ON model_routes
  WHEN NEW.model='reject' BEGIN SELECT RAISE(ABORT, 'test rejection'); END`)
	require.NoError(t, err)
	require.Error(t, repo.DeleteModelRoutes(context.Background(), []string{"model-a", "reject"}))
	rules, err := repo.ListModelRouteRules()
	require.NoError(t, err)
	require.Empty(t, rules)
	fallbacks, err := repo.GetModelFallbacks()
	require.NoError(t, err)
	require.Equal(t, "model-b", fallbacks["model-a"])
}
