package middleware

import (
	"testing"
)

func TestUserQuotaTracker_OverdraftPrevention(t *testing.T) {
	tracker := &UserQuotaTracker{inFlight: make(map[int64]*localQuotaUsage)}

	userID := int64(999)
	balance := 0.0025 // can only accommodate 2 concurrent in-flight requests (2 * 0.001 = 0.002 <= 0.0025)
	reserve := 0.001

	// 1. First request acquires reservation
	if ok, _ := tracker.reserveLocal(userID, balance, reserve, 0); !ok {
		t.Fatalf("first request should acquire reservation")
	}

	// 2. Second request acquires reservation
	if ok, _ := tracker.reserveLocal(userID, balance, reserve, 0); !ok {
		t.Fatalf("second request should acquire reservation")
	}

	// 3. Third concurrent request should be rejected to prevent overdraft!
	// (0.0025 - 2*0.001 = 0.0005, which cannot reserve another 0.001)
	if ok, _ := tracker.reserveLocal(userID, balance, reserve, 0); ok {
		t.Fatalf("third concurrent request should be rejected by overdraft prevention")
	}

	// 4. Once a request finishes and releases, new request can proceed
	tracker.releaseLocal(userID, reserveQuotaUnits(reserve))
	if ok, _ := tracker.reserveLocal(userID, balance, reserve, 0); !ok {
		t.Fatalf("request should succeed after previous request released reservation")
	}

	tracker.releaseLocal(userID, reserveQuotaUnits(reserve))
	tracker.releaseLocal(userID, reserveQuotaUnits(reserve))
}
