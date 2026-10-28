//go:build integration

package distributed

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// B-01 regression against a real Redis server.
//
// Run with:
//
//	export AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:6379/0
//	go test -tags integration ./internal/distributed -run TestInflightQuotaRealRedis -v
//
// The TTL is shortened to milliseconds so the three properties are observable
// without waiting two minutes.
func TestInflightQuotaRealRedisHeartbeatAndRelease(t *testing.T) {
	client := InitRedis(testRedisURL(t))
	if client == nil || !client.IsActive() {
		t.Fatal("InitRedis did not activate")
	}
	rdb := client.GetRDB()

	previousTTL := inflightQuotaTTL
	inflightQuotaTTL = 120 * time.Millisecond
	t.Cleanup(func() { inflightQuotaTTL = previousTTL })

	ctx := context.Background()
	const userID int64 = 424242
	amountKey, countKey := inflightQuotaAmountKeys(userID)
	cleanup := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rdb.Del(delCtx, amountKey, countKey)
	}
	cleanup()
	t.Cleanup(cleanup)

	const reserveCNY = 0.05

	t.Run("heartbeat keeps the reservation alive past the TTL", func(t *testing.T) {
		cleanup()
		ok, err := client.TryReserveQuotaAmount(ctx, userID, 1.0, reserveCNY, 64)
		if err != nil || !ok {
			t.Fatalf("reserve: ok=%v err=%v", ok, err)
		}
		// Booking the reservation refreshes the TTL; assert the reserve itself wrote
		// the millisecond TTL the heartbeat shares.
		if ttl := rdb.PTTL(ctx, amountKey).Val(); ttl <= 0 || ttl > inflightQuotaTTL {
			t.Fatalf("amount key PTTL after reserve = %s, want (0, %s]", ttl, inflightQuotaTTL)
		}

		stop := client.KeepInflightQuotaAlive(userID)
		defer stop()

		// Run for several TTLs with the heartbeat on: without it both keys would be
		// gone by now.
		time.Sleep(6 * inflightQuotaTTL)

		if n, err := rdb.Exists(ctx, amountKey).Result(); err != nil || n != 1 {
			t.Fatalf("amount key disappeared under a live request: n=%d err=%v", n, err)
		}
		if n, err := rdb.Exists(ctx, countKey).Result(); err != nil || n != 1 {
			t.Fatalf("count key disappeared under a live request: n=%d err=%v", n, err)
		}
	})

	t.Run("without a heartbeat the reservation expires (crash path)", func(t *testing.T) {
		cleanup()
		ok, err := client.TryReserveQuotaAmount(ctx, userID, 1.0, reserveCNY, 64)
		if err != nil || !ok {
			t.Fatalf("reserve: ok=%v err=%v", ok, err)
		}
		time.Sleep(3 * inflightQuotaTTL)
		if n, err := rdb.Exists(ctx, amountKey).Result(); err != nil || n != 0 {
			t.Fatalf("an unrefreshed reservation did not expire: exists=%d err=%v", n, err)
		}
	})

	t.Run("stop then release clears both keys and a late tick cannot revive them", func(t *testing.T) {
		cleanup()
		ok, err := client.TryReserveQuotaAmount(ctx, userID, 1.0, reserveCNY, 64)
		if err != nil || !ok {
			t.Fatalf("reserve: ok=%v err=%v", ok, err)
		}
		stop := client.KeepInflightQuotaAlive(userID)
		time.Sleep(2 * inflightQuotaTTL) // let at least one refresh happen
		stop()                           // documented order: stop, then release

		if err := client.ReleaseQuotaAmount(ctx, userID, quotaReserveUnits(reserveCNY)); err != nil {
			t.Fatalf("release: %v", err)
		}
		if n, err := rdb.Exists(ctx, amountKey, countKey).Result(); err != nil || n != 0 {
			t.Fatalf("keys survived release: exists=%d err=%v", n, err)
		}
		time.Sleep(2 * inflightQuotaTTL)
		if n, _ := rdb.Exists(ctx, amountKey, countKey).Result(); n != 0 {
			t.Fatalf("a late heartbeat re-armed the released reservation: exists=%d", n)
		}
	})

	t.Run("keep-alive never re-creates an expired reservation", func(t *testing.T) {
		cleanup()
		stop := client.KeepInflightQuotaAlive(userID)
		defer stop()
		time.Sleep(2 * inflightQuotaHeartbeatInterval())
		if n, _ := rdb.Exists(ctx, amountKey, countKey).Result(); n != 0 {
			t.Fatalf("heartbeat created keys for a reservation that does not exist: exists=%d", n)
		}
	})
}

// TestInflightQuotaRealRedisReleaseCrossesReservations is the regression for the
// release semantics that made the original bug expensive: the keys are per user,
// so a release can only be safe while the reservation still belongs to this
// request. With the heartbeat in place a live request never loses its keys, so the
// dangerous sequence (expiry -> new reservation -> old release) requires a real
// crash/abandonment of the old request.
func TestInflightQuotaRealRedisReleaseDoesNotAffectFreshReservation(t *testing.T) {
	client := InitRedis(testRedisURL(t))
	if client == nil || !client.IsActive() {
		t.Fatal("InitRedis did not activate")
	}
	rdb := client.GetRDB()
	ctx := context.Background()

	const userID int64 = 424243
	amountKey, countKey := inflightQuotaAmountKeys(userID)
	cleanup := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rdb.Del(delCtx, amountKey, countKey)
	}
	cleanup()
	t.Cleanup(cleanup)

	// A reserves and is kept alive.
	if ok, err := client.TryReserveQuotaAmount(ctx, userID, 1.0, 0.05, 64); err != nil || !ok {
		t.Fatalf("reserve A: ok=%v err=%v", ok, err)
	}
	stopA := client.KeepInflightQuotaAlive(userID)
	defer stopA()

	// B (the same user) reserves: the amount aggregates and the count grows.
	if ok, err := client.TryReserveQuotaAmount(ctx, userID, 1.0, 0.02, 64); err != nil || !ok {
		t.Fatalf("reserve B: ok=%v err=%v", ok, err)
	}

	reserved, err := rdb.Get(ctx, amountKey).Int64()
	if err != nil {
		t.Fatalf("amount key missing: %v", err)
	}
	if want := quotaReserveUnits(0.05) + quotaReserveUnits(0.02); reserved != want {
		t.Fatalf("aggregate reservation = %d, want %d", reserved, want)
	}
	if count, _ := rdb.Get(ctx, countKey).Int64(); count != 2 {
		t.Fatalf("in-flight count = %d, want 2", count)
	}

	// A finishes and releases exactly its own booking.
	stopA()
	if err := client.ReleaseQuotaAmount(ctx, userID, quotaReserveUnits(0.05)); err != nil {
		t.Fatalf("release A: %v", err)
	}

	reserved, err = rdb.Get(ctx, amountKey).Int64()
	if err != nil {
		t.Fatalf("amount key disappeared with B still in flight: %v", err)
	}
	if want := quotaReserveUnits(0.02); reserved != want {
		t.Fatalf("reservation after A's release = %d, want B's %d", reserved, want)
	}
	if count, _ := rdb.Get(ctx, countKey).Int64(); count != 1 {
		t.Fatalf("in-flight count after A's release = %d, want 1", count)
	}
	if _, err := rdb.Get(ctx, amountKey).Result(); err == redis.Nil {
		t.Fatal("amount key was deleted although B still holds a reservation")
	}
}
