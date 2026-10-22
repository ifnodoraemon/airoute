package storage

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// usageLogFacetCacheTTL bounds how stale the filter dropdowns may be. Facets
// describe the caller's entire audit history and are independent of the active
// search, so they only change when a new model/channel first appears (or when
// the trail is cleared). Two DISTINCT scans on every console request — including
// the 10-second polls — were pure repetition.
const usageLogFacetCacheTTL = 30 * time.Second

type usageLogFacetCacheEntry struct {
	facets    *UsageLogFacets
	expiresAt time.Time
}

// UsageLogFacets describes the caller's entire audit history, independent of
// pagination and active filters. Historical/deleted channels remain searchable.
type UsageLogFacets struct {
	Models   []string `json:"models"`
	Channels []string `json:"channels"`
}

// usageLogFacetCacheKey covers exactly the inputs ListUsageLogFacets honours:
// the authorization scope. Search filters and pagination are not part of the key
// because the facets deliberately ignore them.
func usageLogFacetCacheKey(f LogFilter) string {
	keys := append([]string(nil), f.APIKeys...)
	sort.Strings(keys)
	return "scoped=" + strconv.FormatBool(f.ScopeByAPIKeys) +
		"|tenant=" + f.TenantID +
		"|keys=" + strings.Join(keys, ",")
}

func (r *Repository) cachedUsageLogFacets(key string) *UsageLogFacets {
	r.facetMu.Lock()
	defer r.facetMu.Unlock()
	entry, ok := r.facetCache[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(r.facetCache, key)
		return nil
	}
	return entry.facets
}

func (r *Repository) storeUsageLogFacets(key string, facets *UsageLogFacets) {
	r.facetMu.Lock()
	defer r.facetMu.Unlock()
	if r.facetCache == nil {
		r.facetCache = make(map[string]usageLogFacetCacheEntry)
	}
	r.facetCache[key] = usageLogFacetCacheEntry{facets: facets, expiresAt: time.Now().Add(usageLogFacetCacheTTL)}
}

// invalidateUsageLogFacetCache drops every cached facet page, e.g. after the
// audit trail is cleared, so the dropdowns empty out immediately.
func (r *Repository) invalidateUsageLogFacetCache() {
	r.facetMu.Lock()
	defer r.facetMu.Unlock()
	r.facetCache = nil
}

// ListUsageLogFacets returns the model/channel domains for the caller's scope.
// The returned value is shared with the cache and must be treated as read-only.
func (r *Repository) ListUsageLogFacets(f LogFilter) (*UsageLogFacets, error) {
	if f.ScopeByAPIKeys && len(f.APIKeys) == 0 {
		return &UsageLogFacets{Models: []string{}, Channels: []string{}}, nil
	}
	cacheKey := usageLogFacetCacheKey(f)
	if cached := r.cachedUsageLogFacets(cacheKey); cached != nil {
		return cached, nil
	}

	result := &UsageLogFacets{Models: []string{}, Channels: []string{}}
	// Keep the authorization scope, discard search and pagination conditions.
	where, args := r.buildUsageLogWhere(LogFilter{
		TenantID: f.TenantID, APIKeys: f.APIKeys, ScopeByAPIKeys: f.ScopeByAPIKeys,
	})
	for _, field := range []struct {
		column string
		values *[]string
	}{{"model", &result.Models}, {"channel", &result.Channels}} {
		rows, err := r.db.Query(`SELECT DISTINCT `+field.column+` FROM usage_logs WHERE COALESCE(`+field.column+`, '') <> ''`+where+` ORDER BY `+field.column, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			*field.values = append(*field.values, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	r.storeUsageLogFacets(cacheKey, result)
	return result, nil
}
