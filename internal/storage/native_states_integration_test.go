//go:build integration

package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresNativeStatesCoordinateReplicasAndExpire(t *testing.T) {
	dsn := partitionScratchDSN(t)
	first, err := OpenDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	repos := []*Repository{NewRepository(first), NewRepository(second)}
	ctx := context.Background()
	s := NativeState{Namespace: "gemini-batch", ID: "batches/shared", Owner: "owner", Identity: "credential", Model: "m", Data: []byte(`{"done":false}`), Billing: []byte(`{"key":"hash"}`), ExpiresAt: time.Now().Add(time.Hour)}
	if err := repos[0].SaveNativeState(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := repos[1].LoadNativeState(ctx, s.Namespace, s.ID, "foreign"); !errors.Is(err, ErrNativeStateNotFound) {
		t.Fatalf("foreign owner saw shared state: %v", err)
	}
	wrong := s
	wrong.Identity = "different"
	if err := repos[1].SaveNativeState(ctx, wrong); err == nil {
		t.Fatal("upstream credential changed")
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			won, err := repos[i%2].ClaimNativeState(ctx, s.Namespace, s.ID, s.Owner)
			if err != nil {
				t.Error(err)
			}
			if won {
				winners.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("cross-replica claim winners=%d", winners.Load())
	}
	if err := repos[1].CompleteNativeState(ctx, s.Namespace, s.ID, s.Owner); err != nil {
		t.Fatal(err)
	}
	if won, err := repos[0].ClaimNativeState(ctx, s.Namespace, s.ID, s.Owner); err != nil || won {
		t.Fatal("completed job became billable again")
	}
	s.ExpiresAt = time.Now().Add(-time.Hour)
	if err := repos[0].SaveNativeState(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := repos[1].LoadNativeState(ctx, s.Namespace, s.ID, s.Owner); !errors.Is(err, ErrNativeStateNotFound) {
		t.Fatal("expired state was visible to peer")
	}
	if err := repos[1].PruneNativeStates(ctx); err != nil {
		t.Fatal(err)
	}
}
