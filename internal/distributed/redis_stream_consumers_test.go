package distributed

import (
	"testing"
	"time"
)

// TestStaleConsumers pins the sweep rule: the sweeper's own entry, consumers that
// still own pending messages and recently active consumers must all survive. A
// non-zero PEL matters because XGROUP DELCONSUMER drops the consumer's pending
// entries without writing them anywhere.
func TestStaleConsumers(t *testing.T) {
	now := 48 * time.Hour
	consumers := []StreamConsumer{
		{Name: "self", Pending: 0, Idle: now},              // the sweeper itself
		{Name: "alive", Pending: 0, Idle: 5 * time.Second}, // active
		{Name: "old-with-pending", Pending: 3, Idle: now},  // unclaimed PEL: keep
		{Name: "old-empty", Pending: 0, Idle: 25 * time.Hour},
		{Name: "boundary", Pending: 0, Idle: 24 * time.Hour}, // >= minIdle: pruned
		{Name: "", Pending: 0, Idle: now},                    // malformed: keep
	}

	got := staleConsumers(consumers, "self", 24*time.Hour)
	want := []string{"boundary", "old-empty"}
	if len(got) != len(want) {
		t.Fatalf("staleConsumers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("staleConsumers = %v, want %v", got, want)
		}
	}
}

// TestStaleConsumersSorted keeps the log line and therefore the tests stable.
func TestStaleConsumersSorted(t *testing.T) {
	consumers := []StreamConsumer{
		{Name: "zeta", Idle: 48 * time.Hour},
		{Name: "alpha", Idle: 48 * time.Hour},
		{Name: "mid", Idle: 48 * time.Hour},
	}
	got := staleConsumers(consumers, "self", time.Hour)
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("staleConsumers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("staleConsumers = %v, want %v", got, want)
		}
	}
}

// TestPruneIdleConsumersInactiveClient documents that the sweep is a no-op when
// the client is not connected, instead of panicking or returning a bogus error.
func TestPruneIdleConsumersInactiveClient(t *testing.T) {
	client := &Client{}
	if _, err := client.PruneIdleConsumers(t.Context(), "stream", "group", "self", time.Hour); err == nil {
		t.Fatal("expected an error from an inactive client")
	}
}
