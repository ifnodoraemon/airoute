package router

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// The channel timeout is the single budget an operator sets; the window a channel
// may stay silent is the same number, not a second, smaller guess.
func TestFirstTokenWindowEqualsChannelTimeout(t *testing.T) {
	dispatcher := NewDispatcher(nil)

	cases := []struct {
		channel *model.ChannelConfig
		want    time.Duration
	}{
		{&model.ChannelConfig{TimeoutSeconds: 300}, 300 * time.Second},
		{&model.ChannelConfig{TimeoutSeconds: 1800}, 1800 * time.Second},
		{&model.ChannelConfig{TimeoutSeconds: 1}, time.Second},
	}
	for _, tc := range cases {
		if got := dispatcher.GetFirstTokenTimeout(tc.channel, "deepseek-v4.1-flash"); got != tc.want {
			t.Fatalf("first-token window = %v, want the channel timeout %v", got, tc.want)
		}
	}

	// Without a configured timeout the historical default still applies.
	if got := dispatcher.GetFirstTokenTimeout(&model.ChannelConfig{}, "plain-model"); got != 30*time.Second {
		t.Fatalf("default window = %v, want 30s", got)
	}
}
