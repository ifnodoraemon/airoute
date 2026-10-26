package main

import (
	"testing"
	"time"
)

func TestEdgeTimeoutBudgetFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_EDGE_TIMEOUT_SEC", "900")
	if got := edgeTimeoutBudget(); got != 900*time.Second {
		t.Fatalf("edge budget = %v, want 15m", got)
	}
	t.Setenv("GATEWAY_EDGE_TIMEOUT_SEC", "not-a-number")
	if got := edgeTimeoutBudget(); got != defaultEdgeTimeoutBudget {
		t.Fatalf("edge budget = %v, want the default on unparsable input", got)
	}
	t.Setenv("GATEWAY_EDGE_TIMEOUT_SEC", "0")
	if got := edgeTimeoutBudget(); got != defaultEdgeTimeoutBudget {
		t.Fatalf("edge budget = %v, want the default on non-positive input", got)
	}
}
