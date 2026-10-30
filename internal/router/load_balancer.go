package router

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// SessionAffinitySelector routes requests using sticky session affinity (Redis cache pin or rendezvous hashing).
type SessionAffinitySelector struct {
	cb *CircuitBreaker
}

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
					// Verify pinned channel is healthy and not tripped.
					// Healthy() is read-only: CanExecute would consume a
					// half-open canary slot just by evaluating candidates.
					if s.cb == nil || s.cb.Healthy(ch.Name) {
						chosen = ch
						chosenIdx = i
					}
					break
				}
			}
		}
	}

	// If not in Redis or pinned channel was tripped, pick with rendezvous
	// (highest-random-weight) hashing.
	if chosen == nil {
		chosen, chosenIdx = rendezvousPick(group, modelName, sessionID, s.cb)

		// Persist pin in Redis (30-minute TTL)
		if client := distributed.GetClient(); client != nil && client.IsActive() {
			go func(m, s, chName string) {
				_ = client.SetSessionChannel(context.Background(), m, s, chName, 30*time.Minute)
			}(modelName, sessionID, chosen.Name)
		}
	}

	telemetry.ScopedLogger("router").Debug("routed via LLM session affinity",
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

		key := swrrKey(modelName, ch.Name)
		cur := s.weights[key] + w
		s.weights[key] = cur
		if cur > maxWeight {
			maxWeight = cur
			winnerIdx = i
		}
	}

	winnerKey := swrrKey(modelName, group[winnerIdx].Name)
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

// rendezvousPick selects one channel with rendezvous (highest-random-weight)
// hashing: every candidate gets a score derived from (session, model, channel),
// and the highest score wins.
//
// This is what makes the affinity actually sticky under configuration changes.
// The previous implementation was `hash(session:model) % len(group)`: adding or
// removing one channel changed the modulus and remapped ~(N-1)/N of all live
// sessions (measured 74.8% for 3→4), flushing the upstream prompt cache of almost
// every conversation. Rendezvous hashing only moves the sessions that the new
// channel wins, i.e. ~1/N (F-06).
//
// A healthy candidate always beats an unhealthy one; when every candidate is
// tripped the highest score is returned so the caller keeps its historical
// behaviour of still attempting one channel.
func rendezvousPick(group []*model.ChannelConfig, modelName, sessionID string, cb *CircuitBreaker) (*model.ChannelConfig, int) {
	bestIdx, bestScore := 0, uint64(0)
	bestHealthyIdx, bestHealthyScore := -1, uint64(0)

	for i, ch := range group {
		score := rendezvousScore(sessionID, modelName, ch.Name)
		if i == 0 || score > bestScore {
			bestIdx, bestScore = i, score
		}
		if cb == nil || cb.Healthy(ch.Name) {
			if bestHealthyIdx < 0 || score > bestHealthyScore {
				bestHealthyIdx, bestHealthyScore = i, score
			}
		}
	}
	if bestHealthyIdx >= 0 {
		return group[bestHealthyIdx], bestHealthyIdx
	}
	return group[bestIdx], bestIdx
}

// rendezvousScore hashes one (session, model, channel) triple into the weight used
// by rendezvousPick. The field separator keeps "ab"+"c" from colliding with
// "a"+"bc"; the splitmix64 finalizer spreads the FNV bits so near-identical inputs
// still compare as unrelated scores.
func rendezvousScore(sessionID, modelName, channelName string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(sessionID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(modelName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(channelName))
	return mixRendezvousBits(h.Sum64())
}

// mixRendezvousBits is the splitmix64 finalizer.
func mixRendezvousBits(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
