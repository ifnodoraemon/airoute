package telemetry

import (
	"sync"
	"sync/atomic"
	"testing"
)

// Regression for the ring-buffer broadcast race.
//
// Append used to copy the subscriber set under the write lock, release the lock
// and only then send. Subscribe's cancel func closes the subscriber channel
// under the same lock, so a cancel landing between the copy and the send made
// the logging goroutine panic with "send on closed channel" — fatal whenever
// that goroutine was not an HTTP handler covered by gin.Recovery.
//
// This test keeps long-lived subscribers so every Append walks a list that is
// long enough for a cancel to land inside the broadcast, then churns
// subscriptions while the writers are appending. Run with -race.
func TestAppendWhileSubscribersCancelDoesNotPanic(t *testing.T) {
	rb := NewLogRingBuffer(64, "test-node")

	// Persistent subscribers widen the broadcast loop (the old code sent to a
	// copied snapshot after unlocking, which is exactly the window under test).
	const persistentSubs = 128
	persistent := make([]func(), 0, persistentSubs)
	var drainWG sync.WaitGroup
	for i := 0; i < persistentSubs; i++ {
		ch, cancel := rb.Subscribe()
		persistent = append(persistent, cancel)
		drainWG.Add(1)
		go func() {
			defer drainWG.Done()
			for range ch {
			}
		}()
	}

	stop := make(chan struct{})
	var writersWG sync.WaitGroup
	var panics atomic.Int64
	for w := 0; w < 3; w++ {
		writersWG.Add(1)
		go func() {
			defer writersWG.Done()
			defer func() {
				if r := recover(); r != nil {
					panics.Add(1)
					t.Errorf("Append panicked while a subscriber cancelled: %v", r)
				}
			}()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rb.Append(&SystemLogEntry{Message: "churn", Level: "INFO"})
			}
		}()
	}

	// Churn: each cancel closes a channel while the writers broadcast.
	for i := 0; i < 400; i++ {
		ch, cancel := rb.Subscribe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range ch {
			}
		}()
		cancel()
		<-done
	}
	close(stop)
	writersWG.Wait()

	for _, cancel := range persistent {
		cancel()
	}
	drainWG.Wait()

	if got := panics.Load(); got != 0 {
		t.Fatalf("Append panicked %d times after a subscriber cancelled", got)
	}
	if entries := rb.GetRecentLogs(1, "", "", "", ""); len(entries) != 1 {
		t.Fatalf("buffer stopped recording entries, got %d", len(entries))
	}
}

// A cancelled subscription must be dropped from the broadcast set so a later
// Append neither panics nor delivers to the closed channel.
func TestAppendAfterCancelDoesNotPanic(t *testing.T) {
	rb := NewLogRingBuffer(8, "test-node")

	ch, cancel := rb.Subscribe()
	cancel()
	// Second cancel must be a no-op (the SSE handler defers it exactly once, but
	// keeping it idempotent protects future call sites).
	cancel()

	rb.Append(&SystemLogEntry{Message: "after-cancel", Level: "INFO"})

	if _, ok := <-ch; ok {
		t.Fatal("closed subscriber channel must not deliver entries")
	}
	entries := rb.GetRecentLogs(1, "", "", "", "")
	if len(entries) != 1 || entries[0].Message != "after-cancel" {
		t.Fatalf("buffer did not record the entry: %+v", entries)
	}
}
