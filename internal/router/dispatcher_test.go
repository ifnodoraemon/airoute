package router

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestDispatcherCandidateResolutionAndDecorators(t *testing.T) {
	channels := []model.ChannelConfig{
		{
			ID:        1,
			Name:      "ch-chat-only",
			Type:      model.ProviderOpenAI,
			Status:    "active",
			Models:    []string{"gpt-4o"},
			Protocols: []string{"chat"},
			Priority:  1,
			Weight:    100,
		},
		{
			ID:        2,
			Name:      "ch-embed-only",
			Type:      model.ProviderOpenAI,
			Status:    "active",
			Models:    []string{"text-embedding-3-small"},
			Protocols: []string{"embeddings"},
			Priority:  1,
			Weight:    100,
		},
		{
			ID:        3,
			Name:      "ch-general",
			Type:      model.ProviderOpenAI,
			Status:    "active",
			Models:    []string{"gpt-4o", "text-embedding-3-small"},
			Protocols: nil, // supports all protocols generally
			Priority:  2,
			Weight:    50,
		},
	}

	d := NewDispatcher(channels)

	t.Run("ResolveCandidateChannels with exact protocol", func(t *testing.T) {
		matched := d.ResolveCandidateChannels(context.Background(), "gpt-4o", "chat")
		if len(matched) == 0 {
			t.Fatalf("expected at least 1 channel for gpt-4o chat")
		}
		// First candidate should be priority 1 (ch-chat-only)
		if matched[0].Name != "ch-chat-only" {
			t.Fatalf("expected ch-chat-only as primary, got %s", matched[0].Name)
		}
	})

	t.Run("ResolveCandidateChannels fallback to general when protocol unmatched", func(t *testing.T) {
		// Model gpt-4o does not have an "images" specific channel, should fallback to general channel (ch-general)
		matched := d.ResolveCandidateChannels(context.Background(), "gpt-4o", "images")
		if len(matched) != 1 || matched[0].Name != "ch-general" {
			t.Fatalf("expected fallback to ch-general, got %v", matched)
		}
	})

	t.Run("ResolveCandidateChannels not found", func(t *testing.T) {
		matched := d.ResolveCandidateChannels(context.Background(), "non-existent-model", "chat")
		if len(matched) != 0 {
			t.Fatalf("expected 0 matches, got %d", len(matched))
		}
	})

	t.Run("recordChannelFailure and recordChannelSuccess", func(t *testing.T) {
		ctx := context.Background()
		var attempted []string

		// Channel 1 fails
		name := d.recordChannelFailure("ch-1", errors.New("timeout"), 1)
		attempted = append(attempted, name)

		if len(attempted) != 1 || attempted[0] != "ch-1" {
			t.Fatalf("unexpected attempted list: %v", attempted)
		}

		// Channel 2 succeeds
		trail := d.recordChannelSuccess(ctx, attempted, "ch-2", 150*time.Millisecond, 10, 20)
		if trail != "ch-1→ch-2" {
			t.Fatalf("expected trail 'ch-1→ch-2', got '%s'", trail)
		}
	})

	t.Run("formatChannelTrail long trail truncation", func(t *testing.T) {
		ctx := context.Background()
		attempted := []string{"ch-alpha", "ch-beta", "ch-gamma"}
		trail := formatChannelTrail(ctx, attempted, "ch-final")
		if trail != "ch-alpha→ch-beta→ch-gamma→ch-final" {
			t.Fatalf("unexpected trail: %s", trail)
		}

		// Simulate extremely long trail exceeding 128 runes
		longAttempts := make([]string, 20)
		for i := range longAttempts {
			longAttempts[i] = "super-long-channel-identifier-segment"
		}
		truncated := formatChannelTrail(ctx, longAttempts, "final-serving-channel")
		if !strings.HasPrefix(truncated, "…") {
			t.Fatalf("expected ellipsis prefix for truncated trail, got: %s", truncated)
		}
		if !strings.Contains(truncated, "final-serving-channel") {
			t.Fatalf("expected final serving channel to survive in truncated trail: %s", truncated)
		}
	})
}
