package model

import "testing"

// R-17: the moderation handler asks for protocol "moderation", but SupportsProtocol
// had no alias group for it, so every explicit capability list failed to match and
// the dispatcher silently fell back to unrestricted channels.
func TestP2ModerationProtocolAliasMatches(t *testing.T) {
	for _, declared := range []string{"moderation", "moderations"} {
		ch := &ChannelConfig{Protocols: []string{declared}}
		if !ch.SupportsProtocol("moderation") {
			t.Fatalf("channel declaring %q does not support the moderation protocol", declared)
		}
		if !ch.SupportsProtocol("moderations") {
			t.Fatalf("channel declaring %q does not support the moderations alias", declared)
		}
	}
	// Existing alias behaviour must be preserved.
	ch := &ChannelConfig{Protocols: []string{"openai_chat"}}
	if !ch.SupportsProtocol("chat") || !ch.SupportsProtocol("responses") {
		t.Fatalf("chat/response alias handling regressed: %+v", ch.Protocols)
	}
}

// A channel that declares only chat must not be considered moderation-capable: that
// is the hole the missing alias caused (fallback selected everyone).
func TestP2ChatChannelIsNotModerationCapable(t *testing.T) {
	ch := &ChannelConfig{Protocols: []string{"openai_chat"}}
	if ch.SupportsProtocol("moderation") {
		t.Fatal("a chat-only channel must not claim moderation capability")
	}
}
