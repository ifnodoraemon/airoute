package router

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// F-06 regression: session affinity is what keeps a conversation on one upstream
// so its prompt cache stays warm. The selector used `fnv(session:model) % N`,
// which remapped ~(N-1)/N of all sessions whenever a channel was added or removed
// (measured 74.8% for 3→4 channels). Rendezvous hashing must only move the
// sessions the new channel wins, i.e. ~1/N.
func TestSessionAffinityRemapsFewSessionsOnChannelAdd(t *testing.T) {
	groupOf := func(n int) []*model.ChannelConfig {
		group := make([]*model.ChannelConfig, 0, n)
		for i := 0; i < n; i++ {
			group = append(group, &model.ChannelConfig{ID: int64(i + 1), Name: fmt.Sprintf("channel-%d", i+1), Weight: 1, Status: "active"})
		}
		return group
	}
	selector := &SessionAffinitySelector{}
	chosenName := func(group []*model.ChannelConfig, sessionID, modelName string) string {
		picked := selector.Select(context.Background(), group, modelName, sessionID)
		if len(picked) == 0 {
			t.Fatalf("selector returned no channel")
		}
		return picked[0].Name
	}

	const sessions = 2000
	const modelName = "deepseek-v4.1-flash"

	cases := []struct {
		name   string
		before int
		after  int
	}{
		{"3 to 4 channels", 3, 4},
		{"4 to 5 channels", 4, 5},
		{"4 to 3 channels (removal)", 4, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, after := groupOf(tc.before), groupOf(tc.after)
			remapped := 0
			for i := 0; i < sessions; i++ {
				sessionID := fmt.Sprintf("session-%d", i)
				if chosenName(before, sessionID, modelName) != chosenName(after, sessionID, modelName) {
					remapped++
				}
			}
			rate := float64(remapped) / float64(sessions)
			t.Logf("%s remapped %.1f%% of sessions", tc.name, rate*100)
			// Theoretical ~1/max(N) = 25% here; allow headroom for hash variance
			// but stay far below the old modulo behaviour (>=65%).
			if rate >= 0.40 {
				t.Fatalf("remapped %.1f%% of sessions, want < 40%% (modulo-N behaviour was 74.8%%)", rate*100)
			}
		})
	}
}

// A session must stay on the same channel for as long as the candidate set and
// health state do not change, and different sessions must spread across channels.
func TestSessionAffinityIsStickyAndSpread(t *testing.T) {
	group := []*model.ChannelConfig{
		{ID: 1, Name: "a", Weight: 1, Status: "active"},
		{ID: 2, Name: "b", Weight: 1, Status: "active"},
		{ID: 3, Name: "c", Weight: 1, Status: "active"},
	}
	selector := &SessionAffinitySelector{}
	pick := func(sessionID string) string {
		picked := selector.Select(context.Background(), group, "m", sessionID)
		if len(picked) != len(group) {
			t.Fatalf("selector dropped candidates: %d", len(picked))
		}
		if picked[0].Name == "" {
			t.Fatal("empty choice")
		}
		return picked[0].Name
	}

	counts := map[string]int{}
	for i := 0; i < 600; i++ {
		sessionID := fmt.Sprintf("s-%d", i)
		first := pick(sessionID)
		for attempt := 0; attempt < 3; attempt++ {
			if got := pick(sessionID); got != first {
				t.Fatalf("session %s moved from %s to %s without a config change", sessionID, first, got)
			}
		}
		counts[first]++
	}
	for _, name := range []string{"a", "b", "c"} {
		if counts[name] == 0 {
			t.Fatalf("channel %s received no session (counts=%v)", name, counts)
		}
	}
}

// An unhealthy channel must not be chosen while a healthy one exists.
func TestSessionAffinitySkipsUnhealthyChannels(t *testing.T) {
	group := []*model.ChannelConfig{
		{ID: 1, Name: "a", Weight: 1, Status: "active"},
		{ID: 2, Name: "b", Weight: 1, Status: "active"},
	}
	cb := NewCircuitBreaker(1, 30*time.Second) // 1 failure trips; 30s cooldown
	cb.RecordFailure("a")
	selector := &SessionAffinitySelector{cb: cb}
	for i := 0; i < 100; i++ {
		picked := selector.Select(context.Background(), group, "m", fmt.Sprintf("s-%d", i))
		if picked[0].Name != "b" {
			t.Fatalf("chose unhealthy channel %q", picked[0].Name)
		}
	}
}
