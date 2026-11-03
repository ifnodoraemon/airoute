package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeStatesOwnerIdentityExpiryAndSingleClaim(t *testing.T) {
	repo := newMigrationsRepo(t)
	ctx := context.Background()
	state := NativeState{Namespace: "gemini-batch", ID: "batches/a", Owner: "owner-a", Identity: "credential-a", Model: "m", Data: []byte(`{"done":false}`), Billing: []byte(`{"key":"digest"}`), ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.SaveNativeState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadNativeState(ctx, state.Namespace, state.ID, "owner-b"); !errors.Is(err, ErrNativeStateNotFound) {
		t.Fatalf("foreign state visible: %v", err)
	}
	other := state
	other.Identity = "credential-b"
	if err := repo.SaveNativeState(ctx, other); err == nil {
		t.Fatal("upstream binding changed")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.ClaimNativeState(ctx, state.Namespace, state.ID, state.Owner)
			if err != nil {
				t.Error(err)
			}
			if ok {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("claimed %d times", successes.Load())
	}
	if _, err := repo.db.Exec(`UPDATE native_states SET claimed=? WHERE namespace=?`, time.Now().Add(-time.Minute).Unix(), state.Namespace); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimNativeState(ctx, state.Namespace, state.ID, state.Owner); err != nil || !ok {
		t.Fatalf("expired worker lease did not recover: %v %v", ok, err)
	}
	if err := repo.CompleteNativeState(ctx, state.Namespace, state.ID, state.Owner); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimNativeState(ctx, state.Namespace, state.ID, state.Owner); err != nil || ok {
		t.Fatal("completed job became billable again")
	}
	loaded, err := repo.LoadNativeState(ctx, state.Namespace, state.ID, state.Owner)
	if err != nil || string(loaded.Billing) != string(state.Billing) {
		t.Fatalf("billing context lost: %+v %v", loaded, err)
	}
	state.ExpiresAt = time.Now().Add(-time.Hour)
	if err := repo.SaveNativeState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadNativeState(ctx, state.Namespace, state.ID, state.Owner); !errors.Is(err, ErrNativeStateNotFound) {
		t.Fatal("expired state was returned")
	}
}
