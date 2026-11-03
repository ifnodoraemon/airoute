package billing

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newDeadlockTestEngine() *BillingEngine {
	return &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{
			"default:exact": {Model: "exact"},
		},
		wildcard: []wildcardRule{
			{group: "default", pattern: "gpt*", price: &storage.ModelPriceRecord{Model: "gpt*"}},
		},
		deflt: &storage.ModelPriceRecord{Model: "*"},
	}
}

// TestGetPriceWithGroupNoNestedRLock is a regression test for a deadlock:
// GetPriceWithGroup held a read lock and prefixMatch() acquired it a second
// time. Because sync.RWMutex is not reentrant, a writer (price reload) queued
// between the two acquisitions deadlocked against the reader forever, stalling
// every request that recorded usage.
//
// The test drives readers through the function while a writer repeatedly takes
// the write lock. If the implementation acquires the read lock twice, the first
// write-lock window deadlocks the readers permanently.
func TestGetPriceWithGroupNoNestedRLock(t *testing.T) {
	e := newDeadlockTestEngine()

	var stop atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				if p := e.GetPriceWithGroup("gpt-4o-mini", "default"); p == nil {
					t.Error("expected a wildcard price, got nil")
					return
				}
				if p := e.GetPriceWithGroup("unknown", "default"); p == nil {
					t.Error("expected the default price, got nil")
					return
				}
			}
		}()
	}

	// Writers: a short bounded burst. Each acquisition is a window in which a
	// nested-RLock implementation would deadlock.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := 0; i < 50; i++ {
			e.mu.Lock()
			time.Sleep(2 * time.Millisecond)
			e.mu.Unlock()
			time.Sleep(time.Millisecond)
		}
	}()

	finished := make(chan struct{})
	go func() {
		<-writerDone
		time.Sleep(20 * time.Millisecond)
		stop.Store(true)
		wg.Wait()
		close(finished)
	}()

	select {
	case <-finished:
		// Readers kept making progress across every write-lock window.
	case <-time.After(15 * time.Second):
		stop.Store(true)
		t.Fatal("readers stopped making progress while a writer held the lock (nested RLock regression)")
	}
}

// TestGetPriceWithGroupResolution pins the expected lookup precedence so the
// lock refactor above cannot silently change which price is returned.
func TestGetPriceWithGroupResolution(t *testing.T) {
	e := newDeadlockTestEngine()

	if p := e.GetPriceWithGroup("exact", "default"); p == nil || p.Model != "exact" {
		t.Fatalf("exact match failed: %+v", p)
	}
	if p := e.GetPriceWithGroup("gpt-4o-mini", "default"); p == nil || p.Model != "gpt*" {
		t.Fatalf("wildcard match failed: %+v", p)
	}
	if p := e.GetPriceWithGroup("totally-unknown", "default"); p == nil || p.Model != "*" {
		t.Fatalf("default fallback failed: %+v", p)
	}
}
