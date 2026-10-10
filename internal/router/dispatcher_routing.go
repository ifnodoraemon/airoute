package router

import (
	"context"
	"sort"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ContextKeySession is the context key type for sticky session IDs.
type ContextKeySession string

const ContextKeySessionID ContextKeySession = "airoute_session_id"

// GetChannelsForModel returns matching channels for a given model, sorted by priority.
func (d *Dispatcher) GetChannelsForModel(modelName string) []*model.ChannelConfig {
	return d.GetChannelsForModelAndProtocolWithContext(context.Background(), modelName, "")
}

// GetChannelsForModelAndProtocol delegates to GetChannelsForModelAndProtocolWithContext.
func (d *Dispatcher) GetChannelsForModelAndProtocol(modelName, proto string) []*model.ChannelConfig {
	return d.GetChannelsForModelAndProtocolWithContext(context.Background(), modelName, proto)
}

// GetChannelsForModelAndProtocolWithContext returns matching channels for a given model and protocol.
// If a session ID is present in ctx, it enables LLM Session Affinity (consistent hashing and Redis pin)
// to reuse upstream KV Cache (Prefix Caching) across multi-turn conversations.
func (d *Dispatcher) GetChannelsForModelAndProtocolWithContext(ctx context.Context, modelName, proto string) []*model.ChannelConfig {
	d.mu.RLock()
	var matched []*model.ChannelConfig
	for i := range d.channels {
		ch := &d.channels[i]
		if proto != "" && !ch.SupportsProtocol(proto) {
			continue
		}
		if ch.SupportsModel(modelName) {
			matched = append(matched, ch)
		}
	}
	d.mu.RUnlock()

	if len(matched) <= 1 {
		return matched
	}

	// Extract Session ID for Session Affinity
	var sessionID string
	if ctx != nil {
		if v, ok := ctx.Value(ContextKeySessionID).(string); ok && v != "" {
			sessionID = v
		} else if v, ok := ctx.Value("session_id").(string); ok && v != "" {
			sessionID = v
		}
	}

	// Group channels by Priority
	priorityGroups := make(map[int][]*model.ChannelConfig)
	var priorities []int
	for _, ch := range matched {
		p := ch.Priority
		if len(priorityGroups[p]) == 0 {
			priorities = append(priorities, p)
		}
		priorityGroups[p] = append(priorityGroups[p], ch)
	}
	sort.Ints(priorities)

	var result []*model.ChannelConfig
	d.wrrMu.Lock()
	defer d.wrrMu.Unlock()

	for groupIdx, p := range priorities {
		group := priorityGroups[p]
		if len(group) == 1 {
			result = append(result, group[0])
			continue
		}

		var selector ChannelSelectorStrategy
		if groupIdx == 0 && sessionID != "" {
			selector = &SessionAffinitySelector{}
		} else {
			selector = NewSWRRSelector(d.wrrWeights)
		}
		result = append(result, selector.Select(ctx, group, modelName, sessionID)...)
	}

	return result
}

// ResolveCandidateChannels finds configured channels supporting the model and protocol.
// If no channels match the requested protocol, it falls back to channels supporting the model generally.
func (d *Dispatcher) ResolveCandidateChannels(ctx context.Context, modelName, proto string) []*model.ChannelConfig {
	channels := d.GetChannelsForModelAndProtocolWithContext(ctx, modelName, proto)
	if len(channels) == 0 && proto != "" {
		channels = d.GetChannelsForModelAndProtocolWithContext(ctx, modelName, "")
	}
	return channels
}

// GetAllSupportedModels returns a deduplicated list of all configured models.
func (d *Dispatcher) GetAllSupportedModels() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	modelSet := make(map[string]struct{})
	for _, ch := range d.channels {
		for _, m := range ch.Models {
			modelSet[m] = struct{}{}
		}
		for alias := range ch.ModelMapping {
			modelSet[alias] = struct{}{}
		}
	}

	var list []string
	for m := range modelSet {
		list = append(list, m)
	}
	sort.Strings(list)
	return list
}
