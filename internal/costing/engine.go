package costing

import (
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

const defaultTTL = 30 * time.Second

// Engine resolves supplier cost for the data plane. Supplier prices change far
// less often than requests, so lookups are cached briefly; admin writes call
// Invalidate so an edit is visible immediately.
type Engine struct {
	repo  *storage.Repository
	mu    sync.RWMutex
	cache map[string]cacheEntry
	now   func() time.Time
	ttl   time.Duration
}

type cacheEntry struct {
	inputUnitsPerMillion  int64
	outputUnitsPerMillion int64
	found                 bool
	at                    time.Time
}

var global atomic.Pointer[Engine]

// InitGlobal installs the process-wide engine.
func InitGlobal(repo *storage.Repository) *Engine {
	engine := &Engine{repo: repo, cache: make(map[string]cacheEntry), now: time.Now, ttl: defaultTTL}
	global.Store(engine)
	return engine
}

// Global returns the process-wide engine, or nil when not wired (tests/CLI).
func Global() *Engine { return global.Load() }

// Invalidate drops the cached prices after an admin write.
func (e *Engine) Invalidate() {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.cache = make(map[string]cacheEntry)
	e.mu.Unlock()
}

// CostBasisUnits returns the supplier cost of a request. found=false means no
// active price is configured; callers must persist NULL, not zero.
func (e *Engine) CostBasisUnits(channel, model string, promptTokens, completionTokens int64) (int64, bool) {
	if e == nil || e.repo == nil || channel == "" || model == "" {
		return 0, false
	}
	key := channel + "\x00" + model
	now := e.now()
	e.mu.RLock()
	entry, ok := e.cache[key]
	e.mu.RUnlock()
	if ok && now.Sub(entry.at) < e.ttl {
		if !entry.found {
			return 0, false
		}
		return unitCost(entry.inputUnitsPerMillion, entry.outputUnitsPerMillion, promptTokens, completionTokens), true
	}
	rec, err := e.repo.GetSupplierCost(channel, model)
	if err != nil || rec.Status != "active" {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, false
		}
		e.mu.Lock()
		e.cache[key] = cacheEntry{found: false, at: now}
		e.mu.Unlock()
		return 0, false
	}
	e.mu.Lock()
	e.cache[key] = cacheEntry{
		inputUnitsPerMillion:  rec.InputCostUnitsPerMillion,
		outputUnitsPerMillion: rec.OutputCostUnitsPerMillion,
		found:                 true,
		at:                    now,
	}
	e.mu.Unlock()
	return unitCost(rec.InputCostUnitsPerMillion, rec.OutputCostUnitsPerMillion, promptTokens, completionTokens), true
}

func unitCost(inputUnitsPerMillion, outputUnitsPerMillion, promptTokens, completionTokens int64) int64 {
	return (promptTokens*inputUnitsPerMillion + completionTokens*outputUnitsPerMillion) / 1_000_000
}
