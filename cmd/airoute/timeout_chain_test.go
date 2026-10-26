package main

import (
	"testing"
	"time"

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
