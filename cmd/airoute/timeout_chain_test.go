package main

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// The whole point of the single-knob design: the channel timeout is the value that
// decides when a request stops, so the edge proxy must never be able to cut first.
func TestEdgeProxyTimeoutExceedsMaxChannelTimeout(t *testing.T) {
	maxChannel := time.Duration(model.MaxChannelTimeoutSeconds) * time.Second
	if edgeProxyTimeout <= maxChannel {
		t.Fatalf("edge proxy timeout %v must exceed the maximum channel timeout %v", edgeProxyTimeout, maxChannel)
	}
}

// The startup self-check used to compare only channel ↔ edge and therefore claimed
// the invariant was proven while the Go server's own write deadline (600s in the
// shipped config, 180s in the Helm chart) was the component that actually cut
// non-streaming requests. The check must read the same value the server enforces.
func TestServerWriteTimeoutIsDerivedFromConfig(t *testing.T) {
	cases := []struct {
		name      string
		writeSecs int
		want      time.Duration
	}{
		{"600s shipped config", 600, 10 * time.Minute},
		{"180s helm chart", 180, 3 * time.Minute},
		{"unset means no deadline", 0, 0},
		{"negative is ignored", -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.WriteTimeoutSec = tc.writeSecs
			if got := serverWriteTimeout(cfg); got != tc.want {
				t.Fatalf("serverWriteTimeout(%d) = %v, want %v", tc.writeSecs, got, tc.want)
			}
		})
	}
	if got := serverWriteTimeout(nil); got != 0 {
		t.Fatalf("serverWriteTimeout(nil) = %v, want 0", got)
	}
}

// The warning condition is what turns "we printed the numbers" into "we reported the
// contradiction": a 3600s channel behind the shipped 600s write deadline must warn,
// a 60s channel must not, and a disabled write timeout must never warn (a zero value
// means "no deadline", not "deadline of 0").
func TestWriteTimeoutCutsChannel(t *testing.T) {
	cases := []struct {
		name           string
		channelTimeout time.Duration
		writeTimeout   time.Duration
		want           bool
	}{
		{"channel beyond non-streaming deadline", 3600 * time.Second, 600 * time.Second, true},
		{"channel inside the deadline", 60 * time.Second, 600 * time.Second, false},
		{"equal is not a truncation", 600 * time.Second, 600 * time.Second, false},
		{"disabled write timeout never truncates", 3600 * time.Second, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := writeTimeoutCutsChannel(tc.channelTimeout, tc.writeTimeout); got != tc.want {
				t.Fatalf("writeTimeoutCutsChannel(%v, %v) = %v, want %v", tc.channelTimeout, tc.writeTimeout, got, tc.want)
			}
		})
	}
}
