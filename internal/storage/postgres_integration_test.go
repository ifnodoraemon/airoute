//go:build integration

package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// testPostgresURL returns the DSN for the PostgreSQL integration suite.
//
// A dedicated variable is required instead of DATABASE_URL on purpose: these
// tests create rows in a live database, and inheriting DATABASE_URL would point
// them at whatever deployment the developer happens to have exported.
func testPostgresURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("AIROUTE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AIROUTE_TEST_DATABASE_URL is not set; skipping PostgreSQL integration test")
	}
	return url
}

// itSuffix keeps rows from different runs (and from the two tests below) apart,
// so the suite is re-runnable against a persistent server.
func itSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// TestPostgresMigrationsAndRepositoryRoundTrip proves the PostgreSQL path
// actually executes end to end: dialect detection, the Postgres migration set,
// '?' to '$N' placeholder rebinding, and the repository's read/write queries.
//
// Before this suite existed, the Postgres DDL and every Postgres query were
// never run by CI: the unit suite only ever opened SQLite.
func TestPostgresMigrationsAndRepositoryRoundTrip(t *testing.T) {
	url := testPostgresURL(t)

	db, err := OpenDB(url)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	if got := db.Dialect(); got != "postgres" {
		t.Fatalf("dialect = %q, want %q", got, "postgres")
	}

	// Migrations must be applied and recorded, not silently skipped.
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if applied == 0 {
		t.Fatal("no migrations recorded in schema_migrations")
	}
	t.Logf("postgres migrations recorded: %d", applied)

	repo := NewRepository(db)
	suffix := itSuffix()

	// Channel round trip (BIGSERIAL id, JSON text columns).
	model := "pg-it-model-" + suffix
	ch := &ChannelRecord{
		Name:     "pg-it-chan-" + suffix,
		Type:     "openai",
		BaseURL:  "http://pg.invalid",
		APIKey:   "k",
		Models:   []string{model},
		Priority: 1,
		Weight:   10,
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("CreateChannel on postgres: %v", err)
	}
	if ch.ID == 0 {
		t.Fatal("CreateChannel did not populate the id on postgres")
	}
	gotCh, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("GetChannel on postgres: %v", err)
	}
	if gotCh.Name != ch.Name {
		t.Fatalf("channel name round trip = %q, want %q", gotCh.Name, ch.Name)
	}
	if len(gotCh.Models) != 1 || gotCh.Models[0] != model {
		t.Fatalf("channel models round trip = %v, want [%s]", gotCh.Models, model)
	}

	// Price upsert: the second write must take the ON CONFLICT DO UPDATE branch.
	if err := repo.SaveModelPrice(&ModelPriceRecord{
		Model: model, GroupName: "default", PromptPrice: 10, CompletionPrice: 20, Currency: "CNY",
	}); err != nil {
		t.Fatalf("SaveModelPrice insert on postgres: %v", err)
	}
	if err := repo.SaveModelPrice(&ModelPriceRecord{
		Model: model, GroupName: "default", PromptPrice: 11, CompletionPrice: 22, Currency: "CNY",
	}); err != nil {
		t.Fatalf("SaveModelPrice upsert on postgres: %v", err)
	}
	price, err := repo.GetModelPriceExact(model, "default")
	if err != nil {
		t.Fatalf("GetModelPriceExact on postgres: %v", err)
	}
	if price.PromptPrice != 11 || price.CompletionPrice != 22 {
		t.Fatalf("upsert did not update the existing row: %+v", price)
	}

	// Fallback rule through the bundle.
	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:         model,
		FallbackModel: model + "-fallback",
	}); err != nil {
		t.Fatalf("ApplyModelRouteBundle on postgres: %v", err)
	}
	fbs, err := repo.GetModelFallbacks()
	if err != nil {
		t.Fatalf("GetModelFallbacks on postgres: %v", err)
	}
	if fbs[model] != model+"-fallback" {
		t.Fatalf("fallback round trip = %q, want %q", fbs[model], model+"-fallback")
	}
}

// TestPostgresApplyModelRouteBundleIsAtomic runs the same rollback scenario as
// the SQLite unit test against real PostgreSQL.
//
// It needs its own coverage because the failure semantics differ: PostgreSQL
// aborts the whole transaction after a failed statement (SQLite only aborts the
// statement), so the deferred Rollback takes a different path and the earlier
// writes must still disappear.
func TestPostgresApplyModelRouteBundleIsAtomic(t *testing.T) {
	url := testPostgresURL(t)

	db, err := OpenDB(url)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	suffix := itSuffix()
	model := "pg-atomic-" + suffix

	first := &ChannelRecord{
		Name: "pg-atomic-a-" + suffix, Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "k", Models: []string{model}, Priority: 1, Weight: 10,
	}
	second := &ChannelRecord{
		Name: "pg-atomic-b-" + suffix, Type: "openai", BaseURL: "http://b.invalid",
		APIKey: "k", Models: []string{model}, Priority: 1, Weight: 10,
	}
	for _, ch := range []*ChannelRecord{first, second} {
		if err := repo.CreateChannel(ch); err != nil {
			t.Fatalf("create channel %s: %v", ch.Name, err)
		}
	}

	bundle := &ModelRouteBundle{
		Model:         model,
		FallbackModel: model + "-fallback",
		Price:         &ModelPriceRecord{Model: model, GroupName: "default", PromptPrice: 1, CompletionPrice: 2},
		Channels: []ChannelRoutingUpdate{
			// Step 3a: a legitimate update the transaction applies before the
			// failure.
			{ID: first.ID, AddModels: []string{model + "-extra"}, Priority: 2, Weight: 20},
			// Step 3b: a channel that does not exist, which aborts the change.
			{ID: second.ID + 100000, AddModels: []string{model}},
		},
	}
	if err := repo.ApplyModelRouteBundle(context.Background(), bundle); err == nil {
		t.Fatal("expected the vanished channel to fail the bundle on postgres")
	}

	// Step 2: the price row must not survive the rollback.
	if _, err := repo.GetModelPriceExact(model, "default"); err == nil {
		t.Error("price was persisted despite the transaction failing: step 2 was not rolled back")
	}

	// Step 1: neither may the fallback row.
	fbs, err := repo.GetModelFallbacks()
	if err != nil {
		t.Fatalf("read fallbacks: %v", err)
	}
	if _, ok := fbs[model]; ok {
		t.Error("fallback was persisted despite the transaction failing: step 1 was not rolled back")
	}

	// Step 3a: the update that succeeded before the failure must be undone.
	gotFirst, err := repo.GetChannel(first.ID)
	if err != nil {
		t.Fatalf("reload first channel: %v", err)
	}
	if len(gotFirst.Models) != 1 {
		t.Errorf("first channel model list was not rolled back: %v", gotFirst.Models)
	}
	if gotFirst.Priority != 1 || gotFirst.Weight != 10 {
		t.Errorf("first channel routing fields were not rolled back: priority=%d weight=%d",
			gotFirst.Priority, gotFirst.Weight)
	}
	if gotFirst.BaseURL != "http://a.invalid" {
		t.Errorf("first channel lost a field the route change does not own: base_url=%q", gotFirst.BaseURL)
	}
}

// TestPostgresPatchChannel exercises the dialect-conditional read inside
// PatchChannel. On PostgreSQL the SELECT carries FOR UPDATE, which SQLite
// rejects, so this clause is only ever parsed here: a syntax error in it would
// break every channel update on the clustered deployment while the whole unit
// suite stayed green.
func TestPostgresPatchChannel(t *testing.T) {
	url := testPostgresURL(t)

	db, err := OpenDB(url)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	suffix := itSuffix()

	rec := &ChannelRecord{
		Name: "pg-patch-" + suffix, Type: "openai", BaseURL: "http://patch.invalid",
		APIKey: "credential-v1", Models: []string{"m1"}, Priority: 1, Weight: 10, Status: "active",
	}
	if err := repo.CreateChannel(rec); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// A concurrent writer changes the credential through the full-row path.
	stale, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	concurrent := *stale
	concurrent.APIKey = "credential-v2"
	if err := repo.UpdateChannel(&concurrent); err != nil {
		t.Fatalf("concurrent UpdateChannel: %v", err)
	}

	// The patch must see the row as it is now, not as it was at the stale read.
	updated, err := repo.PatchChannel(rec.ID, func(cur *ChannelRecord) error {
		cur.Name = "pg-patch-renamed-" + suffix
		return nil
	})
	if err != nil {
		t.Fatalf("PatchChannel on postgres: %v", err)
	}
	if updated.APIKey != "credential-v2" {
		t.Errorf("patch worked from a stale read: api_key = %q, want credential-v2", updated.APIKey)
	}
	if updated.Name != "pg-patch-renamed-"+suffix {
		t.Errorf("patch did not apply: name = %q", updated.Name)
	}

	// And it is durable.
	stored, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if stored.Name != "pg-patch-renamed-"+suffix || stored.APIKey != "credential-v2" {
		t.Errorf("stored row = {name:%q api_key:%q}, want the patched name and the concurrent credential",
			stored.Name, stored.APIKey)
	}

	// A missing row must report the sentinel, not a raw driver error.
	if _, err := repo.PatchChannel(-1, func(*ChannelRecord) error { return nil }); !errors.Is(err, ErrChannelNotFound) {
		t.Errorf("err = %v, want ErrChannelNotFound for a missing row", err)
	}
}

// TestPostgresPatchChannelSerializesConcurrentWriters proves the FOR UPDATE in
// PatchChannel is load-bearing rather than decorative.
//
// Without it the second writer's SELECT reads the row as it was before the first
// transaction committed. Its UPDATE then waits on the row lock and, once the
// first commits, writes the whole row from that stale read — losing the first
// writer's change even though the two transactions never ran concurrently.
func TestPostgresPatchChannelSerializesConcurrentWriters(t *testing.T) {
	url := testPostgresURL(t)

	db, err := OpenDB(url)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	suffix := itSuffix()

	rec := &ChannelRecord{
		Name: "pg-lock-" + suffix, Type: "openai", BaseURL: "http://lock.invalid",
		APIKey: "k", Models: []string{"m1"}, Priority: 1, Weight: 10, Status: "active",
	}
	if err := repo.CreateChannel(rec); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	holding := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		_, err := repo.PatchChannel(rec.ID, func(cur *ChannelRecord) error {
			cur.Name = "first-writer-" + suffix
			close(holding)
			<-release
			return nil
		})
		firstDone <- err
	}()

	select {
	case <-holding:
	case <-time.After(10 * time.Second):
		t.Fatal("the first writer never entered its mutation")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := repo.PatchChannel(rec.ID, func(cur *ChannelRecord) error {
			cur.Weight = 99
			return nil
		})
		secondDone <- err
	}()

	// While the first transaction holds the row, the second must not reach its
	// mutation. This is a smoke check: the assertion below is the real one.
	select {
	case err := <-secondDone:
		t.Fatalf("the second writer completed while the row was locked: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first PatchChannel: %v", err)
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second PatchChannel: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second writer never completed after the lock was released")
	}

	// Both changes must survive. This fails without FOR UPDATE: the second writer
	// would rewrite the name from its pre-commit read.
	stored, err := repo.GetChannel(rec.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if stored.Name != "first-writer-"+suffix {
		t.Errorf("the first writer's name was lost: %q", stored.Name)
	}
	if stored.Weight != 99 {
		t.Errorf("the second writer's weight was lost: %d", stored.Weight)
	}
}

// TestPostgresKeepsConcurrentModelAdds is the reported race on the dialect that
// serves production traffic.
//
// Two administrators add different models to the same shared channel, each planning
// from the same read. A replacement list would let the second save drop the first
// administrator's model; a delta applied inside the transaction keeps both. The
// SQLite test proves the logic, this proves the PostgreSQL path — including the
// FOR UPDATE read that only exists there.
func TestPostgresKeepsConcurrentModelAdds(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	ch := &ChannelRecord{
		Name: "pg-shared-" + itSuffix(), Type: "openai", BaseURL: "http://pg.invalid",
		APIKey: "k", Models: []string{"pg-base"}, Priority: 1, Weight: 10, Status: "active",
	}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}

	// Both administrators read the channel before either commits.
	if _, err := repo.GetChannel(ch.ID); err != nil {
		t.Fatalf("GetChannel: %v", err)
	}

	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:    "pg-model-a",
		Channels: []ChannelRoutingUpdate{{ID: ch.ID, AddModels: []string{"pg-model-a"}}},
	}); err != nil {
		t.Fatalf("apply A: %v", err)
	}

	if err := repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{
		Model:    "pg-model-b",
		Channels: []ChannelRoutingUpdate{{ID: ch.ID, AddModels: []string{"pg-model-b"}}},
	}); err != nil {
		t.Fatalf("apply B: %v", err)
	}

	stored, err := repo.GetChannel(ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	got := map[string]bool{}
	for _, m := range stored.Models {
		got[m] = true
	}
	for _, want := range []string{"pg-base", "pg-model-a", "pg-model-b"} {
		if !got[want] {
			t.Errorf("model %q was lost on postgres; channel serves %v", want, stored.Models)
		}
	}
}
