package distributed

// Unit tests for the consumer-group-missing classification that the audit-log
// consumer uses to decide "rebuild the group" versus "retry the read".
//
// This is a pure error-classification test: it needs no Redis. The integration
// behaviour (a deleted group is recreated and ingestion resumes) is covered by
// internal/storage's AIROUTE_TEST_REDIS_URL-gated tests.

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsConsumerGroupMissing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", errors.New("connection refused"), false},
		{"redis NOGROUP", errors.New("NOGROUP No such key 'airoute:stream:logs' or consumer group 'airoute_log_group' in XREADGROUP with GROUP option"), true},
		{"lowercase no such key", errors.New("ERR no such key"), true},
		{"wrapped NOGROUP", fmt.Errorf("xreadgroup: %w", errors.New("NOGROUP No such key")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsConsumerGroupMissing(tc.err); got != tc.want {
				t.Fatalf("IsConsumerGroupMissing(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// An inactive client must fail fast instead of pretending the group is healthy;
// the consumer relies on that to decide whether it may keep reading.
func TestConsumerGroupStatsInactiveClient(t *testing.T) {
	c := &Client{}
	if _, err := c.ConsumerGroupStats(context.Background(), "s", "g"); err == nil {
		t.Fatal("expected an error from an inactive client")
	}
	if err := c.DeadLetterLog(context.Background(), "d", "1-0", "reason", "payload", 10); err == nil {
		t.Fatal("expected an error from an inactive client")
	}
}
