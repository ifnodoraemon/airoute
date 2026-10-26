package telemetry

import (
	"log/slog"
	"testing"
)

func newTestSystemLogger(t *testing.T) (*LogRingBuffer, *slog.Logger) {
	t.Helper()
	buffer := NewLogRingBuffer(10, "test-node")
	return buffer, slog.New(NewBroadcastHandler(nil, buffer, slog.LevelDebug))
}

// A subsystem that names its category explicitly must win over the message-text
// heuristic; the heuristic exists only for unlabelled call sites.
func TestBroadcastHandlerPrefersExplicitCategory(t *testing.T) {
	buffer, logger := newTestSystemLogger(t)

	logger.With("category", "billing").Info("plain message with no subsystem keyword")

	entries := buffer.GetRecentLogs(10, "", "", "", "")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Category != "billing" {
		t.Fatalf("explicit category ignored, got %q", entries[0].Category)
	}
	if _, repeated := entries[0].Attrs["category"]; repeated {
		t.Fatal("the category marker leaked into the rendered attributes")
	}
}

func TestBroadcastHandlerFallsBackToMessageHeuristic(t *testing.T) {
	buffer, logger := newTestSystemLogger(t)

	logger.Info("circuit breaker is OPEN, bypassing dead upstream")

	entries := buffer.GetRecentLogs(10, "", "", "", "")
	if len(entries) != 1 || entries[0].Category != "breaker" {
		t.Fatalf("fallback heuristic did not classify the message: %+v", entries)
	}
}

// A typo must not invent a category the console cannot filter on.
func TestBroadcastHandlerIgnoresUnknownExplicitCategory(t *testing.T) {
	buffer, logger := newTestSystemLogger(t)

	logger.With("category", "not-a-real-category").Info("redis sync reload completed")

	entries := buffer.GetRecentLogs(10, "", "", "", "")
	if len(entries) != 1 || entries[0].Category != "sync" {
		t.Fatalf("unknown category should fall back to the heuristic, got %+v", entries)
	}
}

// Attributes attached through With/WithAttrs belong in the captured entry too;
// only the stdout handler used to see them.
func TestBroadcastHandlerKeepsScopedAttributes(t *testing.T) {
	buffer, logger := newTestSystemLogger(t)

	logger.With("category", "router", "channel", "罗总").Info("retry scheduled")

	entries := buffer.GetRecentLogs(10, "", "", "", "")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Attrs["channel"] != "罗总" {
		t.Fatalf("scoped attribute missing from the entry: %+v", entries[0].Attrs)
	}
}

// Access lines are persisted as audit rows; they must not evict the rare events
// an operator opens the console to find.
func TestBroadcastHandlerKeepsAccessLinesOutOfRingBuffer(t *testing.T) {
	buffer, logger := newTestSystemLogger(t)

	logger.With("category", "system").Info("http_access", "status", 200)
	logger.With("category", "billing").Warn("billing event")

	entries := buffer.GetRecentLogs(10, "", "", "", "")
	if len(entries) != 1 {
		t.Fatalf("expected only the billing event in the buffer, got %d entries", len(entries))
	}
	if entries[0].Message != "billing event" {
		t.Fatalf("unexpected entry kept: %+v", entries[0])
	}
}
