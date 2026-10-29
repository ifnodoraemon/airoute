package storage

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestStreamConsumerIDUsesNodeID pins the reason the identity exists: a pinned
// NODE_ID must be used verbatim, without a pid suffix, so a restart reuses the
// same Redis consumer entry instead of adding a new one.
func TestStreamConsumerIDUsesNodeID(t *testing.T) {
	t.Setenv("NODE_ID", " gateway-1 ")
	if got := streamConsumerID(); got != "gateway-1" {
		t.Fatalf("streamConsumerID = %q, want the trimmed NODE_ID", got)
	}
}

// TestStreamConsumerIDFallsBackToHostPID keeps ad-hoc runs isolated from each
// other: two processes on one host must not share a consumer name, because they
// would read each other's pending entries.
func TestStreamConsumerIDFallsBackToHostPID(t *testing.T) {
	t.Setenv("NODE_ID", "")
	got := streamConsumerID()
	if strings.TrimSpace(got) == "" {
		t.Fatal("streamConsumerID returned an empty identity")
	}
	if !strings.HasSuffix(got, fmt.Sprintf("-%d", os.Getpid())) {
		t.Fatalf("streamConsumerID = %q, want a host-pid suffix", got)
	}
}
