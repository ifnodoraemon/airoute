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

// TryAcquire attempts to reserve quota for an in-flight request.
func (t *UserQuotaTracker) TryAcquire(userID int64, balance float64) bool {
	if userID <= 0 {
		return true
	}

	// Cluster distributed check via Redis if active
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		acquired, err := rClient.TryAcquireInFlightQuota(ctx, userID, balance, t.minReserveCost)
		if err == nil {
			return acquired
		}
		// On Redis failure, fall back to local memory below
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	inFlight := t.inFlightUsers[userID]
	if balance-float64(inFlight+1)*t.minReserveCost < 0 {
		return false
	}
	t.inFlightUsers[userID] = inFlight + 1
	return true
}

// Release frees the reserved in-flight quota for a user.
func (t *UserQuotaTracker) Release(userID int64) {
	if userID <= 0 {
		return
	}

	// Cluster distributed release via Redis if active
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		if err := rClient.ReleaseInFlightQuota(ctx, userID); err == nil {
			return
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inFlightUsers[userID] > 1 {
		t.inFlightUsers[userID]--
	} else {
		delete(t.inFlightUsers, userID)
	}
}
