package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
)

// UserQuotaTracker manages in-flight request reserves to prevent concurrent overdraft attacks.
type UserQuotaTracker struct {
	mu             sync.Mutex
	inFlightUsers  map[int64]int
	minReserveCost float64
}

var globalQuotaTracker = &UserQuotaTracker{
	inFlightUsers:  make(map[int64]int),
	minReserveCost: 0.0005, // minimal reserved credit per in-flight request
}

// QuotaBackend identifies where an in-flight reservation was taken, so the
// matching release always targets the same counter.
type QuotaBackend string

const (
	QuotaBackendNone  QuotaBackend = ""
	QuotaBackendRedis QuotaBackend = "redis"
	QuotaBackendLocal QuotaBackend = "local"
)

// TryAcquire attempts to reserve quota for an in-flight request. It returns the
// backend that granted the reservation; the caller must pass it to Release.
func (t *UserQuotaTracker) TryAcquire(userID int64, balance float64) (bool, QuotaBackend) {
	if userID <= 0 {
		return true, QuotaBackendNone
	}

	// Cluster distributed check via Redis if active
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		acquired, err := rClient.TryAcquireInFlightQuota(ctx, userID, balance, t.minReserveCost)
		if err == nil {
			return acquired, QuotaBackendRedis
		}
		// On Redis failure, fall back to local memory below
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	inFlight := t.inFlightUsers[userID]
	if balance-float64(inFlight+1)*t.minReserveCost < 0 {
		return false, QuotaBackendNone
	}
	t.inFlightUsers[userID] = inFlight + 1
	return true, QuotaBackendLocal
}

// Release frees the reserved in-flight quota for a user on the backend that
// granted it. Releasing a different backend would leak the original counter.
func (t *UserQuotaTracker) Release(userID int64, backend QuotaBackend) {
	if userID <= 0 {
		return
	}

	if backend == QuotaBackendRedis {
		rClient := distributed.GetClient()
		if rClient != nil && rClient.IsActive() {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			_ = rClient.ReleaseInFlightQuota(ctx, userID)
		}
		return
	}
	if backend != QuotaBackendLocal {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inFlightUsers[userID] > 1 {
		t.inFlightUsers[userID]--
	} else {
		delete(t.inFlightUsers, userID)
	}
}
