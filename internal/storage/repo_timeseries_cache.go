package storage

import (
	"sync"
	"time"
)

const maxTsCacheEntries = 128

var (
	tsCacheMu sync.RWMutex
	tsCache   = make(map[string]struct {
		data *TimeSeriesData
		at   time.Time
	})
)

func cachedTimeSeries(cacheKey string) *TimeSeriesData {
	tsCacheMu.RLock()
	defer tsCacheMu.RUnlock()
	if entry, ok := tsCache[cacheKey]; ok && time.Since(entry.at) < 3*time.Second {
		return entry.data
	}
	return nil
}

func cacheTimeSeries(cacheKey string, res *TimeSeriesData) {
	tsCacheMu.Lock()
	if len(tsCache) >= maxTsCacheEntries {
		// Prune expired entries
		nowTime := time.Now()
		for k, v := range tsCache {
			if nowTime.Sub(v.at) > 3*time.Second {
				delete(tsCache, k)
			}
		}
		// If still full, purge all
		if len(tsCache) >= maxTsCacheEntries {
			tsCache = make(map[string]struct {
				data *TimeSeriesData
				at   time.Time
			})
		}
	}
	tsCache[cacheKey] = struct {
		data *TimeSeriesData
		at   time.Time
	}{data: res, at: time.Now()}
	tsCacheMu.Unlock()

}
