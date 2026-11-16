package billing

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// resolvePriceWithSource is the single price-resolution implementation; source
// identifies which rule matched so callers can report it.
func (e *BillingEngine) resolvePriceWithSource(modelName, groupName string) (*storage.ModelPriceRecord, string) {
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	grp := strings.ToLower(strings.TrimSpace(groupName))
	if grp == "" {
		grp = "default"
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Exact match in requested group
	if p, ok := e.prices[grp+":"+modelLower]; ok {
		return p, PriceSourceExact
	}

	// 2. Exact match in default group fallback
	if grp != "default" {
		if p, ok := e.prices["default:"+modelLower]; ok {
			return p, PriceSourceExact
		}
		if p, ok := e.prices[modelLower]; ok {
			return p, PriceSourceExact
		}
	} else {
		if p, ok := e.prices[modelLower]; ok {
			return p, PriceSourceExact
		}
	}

	// 3. Wildcard pattern match in group (e.g. "vip:gpt-4o-*" matching "gpt-4o-mini",
	// or "*o1" matching "openai/o1"). The rule list is pre-sorted most-specific
	// first, so the result is stable across requests and processes.
	//
	// NOTE: this closure must NOT re-acquire e.mu. The caller already holds the
	// read lock, and sync.RWMutex is not reentrant: if a writer (price reload)
	// queues between the outer RLock and this one, the inner RLock blocks
	// forever while the writer waits for the outer lock to be released.
	patternMatch := func(g string) *storage.ModelPriceRecord {
		for _, rule := range e.wildcard {
			if rule.group != g {
				continue
			}
			if MatchModelPattern(rule.pattern, modelLower) {
				return rule.price
			}
		}
		return nil
	}

	if p := patternMatch(grp); p != nil {
		return p, PriceSourceWildcard
	}
	if grp != "default" {
		if p := patternMatch("default"); p != nil {
			return p, PriceSourceWildcard
		}
	}

	// 4. Default standard fallback
	return e.deflt, PriceSourceDefault
}