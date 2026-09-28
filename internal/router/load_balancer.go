package router

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// ChannelSelectorStrategy defines the strategy interface for ordering candidate channels within a priority tier.
type ChannelSelectorStrategy interface {
	Select(ctx context.Context, group []*model.ChannelConfig, modelName, sessionID string) []*model.ChannelConfig
}

// SessionAffinitySelector routes requests using sticky session affinity (Redis cache pin or Consistent Hashing).
type SessionAffinitySelector struct{}

// Select implements ChannelSelectorStrategy with session affinity.
func (s *SessionAffinitySelector) Select(ctx context.Context, group []*model.ChannelConfig, modelName, sessionID string) []*model.ChannelConfig {
	if sessionID == "" || len(group) <= 1 {
		return group
	}

	var chosen *model.ChannelConfig
	chosenIdx := -1

	// Check Distributed Redis cache pin first
	if client := distributed.GetClient(); client != nil && client.IsActive() {
		if pinnedName, err := client.GetSessionChannel(ctx, modelName, sessionID); err == nil && pinnedName != "" {
			for i, ch := range group {
				if ch.Name == pinnedName {
					chosen = ch
					chosenIdx = i
					break
				}
			}
		}
	}

	// If not in Redis, compute Consistent Hash via FNV-1a
	if chosen == nil {
		h := fnv.New32a()
		_, _ = h.Write([]byte(sessionID))
		_, _ = h.Write([]byte(":"))
		_, _ = h.Write([]byte(modelName))
		chosenIdx = int(h.Sum32() % uint32(len(group)))
		chosen = group[chosenIdx]

		// Persist pin in Redis (30-minute TTL)
		if client := distributed.GetClient(); client != nil && client.IsActive() {
			go func(m, s, chName string) {
				_ = client.SetSessionChannel(context.Background(), m, s, chName, 30*time.Minute)
			}(modelName, sessionID, chosen.Name)
		}
	}

	telemetry.Logger.Debug("routed via LLM session affinity",
		"session_id", sessionID,
		"model", modelName,
		"channel", chosen.Name,
	)

	result := make([]*model.ChannelConfig, 0, len(group))
	result = append(result, chosen)
	for i, ch := range group {
		if i != chosenIdx {
			result = append(result, ch)
		}
	}
	return result
}

// SWRRSelector implements Smooth Weighted Round-Robin load balancing.
type SWRRSelector struct {
	weights map[string]int
}

// NewSWRRSelector creates a new SWRR selector strategy with backing weights map.
func NewSWRRSelector(weights map[string]int) *SWRRSelector {
	return &SWRRSelector{weights: weights}
}

// Select implements ChannelSelectorStrategy with Smooth Weighted Round-Robin.
func (s *SWRRSelector) Select(ctx context.Context, group []*model.ChannelConfig, modelName, sessionID string) []*model.ChannelConfig {
	if len(group) <= 1 {
		return group
	}

	totalWeight := 0
	maxWeight := -1 << 31
	winnerIdx := 0

	for i, ch := range group {
		w := ch.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w

		key := modelName + ":" + ch.Name
		cur := s.weights[key] + w
		s.weights[key] = cur
		if cur > maxWeight {
			maxWeight = cur
			winnerIdx = i
		}
	}

	winnerKey := modelName + ":" + group[winnerIdx].Name
	s.weights[winnerKey] -= totalWeight

	result := make([]*model.ChannelConfig, 0, len(group))
	// Winner is tried first
	result = append(result, group[winnerIdx])
	// Remaining channels in this tier serve as immediate fallbacks
	for i, ch := range group {
		if i != winnerIdx {
			result = append(result, ch)
		}
	}
	return result
}
