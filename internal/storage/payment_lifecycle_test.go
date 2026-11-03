package storage

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPaymentStorageLifecycle(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	paymentStorageSuite(t, db)
}

// The same money invariants run on SQLite and real PostgreSQL.
func paymentStorageSuite(t *testing.T, db *DB) {
	r := NewRepository(db)
	suffix := fmt.Sprint(time.Now().UnixNano())
	newUser := func(name string) string {
		name += "-" + suffix
		require.NoError(t, r.CreateUser(&UserRecord{Username: name, PasswordHash: "synthetic", Role: "user", Status: "active"}))
		return name
	}
	newOrder := func(name, user string, amount float64) *RechargeOrderRecord {
		o, err := r.CreateRechargeIntent(&RechargeOrderRecord{OrderNo: name + "-" + suffix, Username: user, Amount: amount, Currency: "CNY", IdempotencyKey: name + "-" + suffix, PaymentMode: "test", SuccessURL: "https://wallet.example.invalid/return", CancelURL: "https://wallet.example.invalid/cancel", ExpiresAt: time.Now().Add(time.Hour).Unix()})
		require.NoError(t, err)
		return o
	}
	balance := func(user string) int64 {
		u, err := r.GetUserByUsername(user)
		require.NoError(t, err)
		return u.BalanceUnits
	}
	snapshot := func(o *RechargeOrderRecord) PaymentSnapshot {
		return PaymentSnapshot{SessionID: "cs_" + o.OrderNo, PaymentIntentID: "pi_" + o.OrderNo, Mode: "test", Currency: "CNY", Status: "paid", AmountCents: o.Cents(), ExpiresAt: o.ExpiresAt}
	}
	apply := func(o *RechargeOrderRecord, s PaymentSnapshot) error {
		var claimed *RechargeOrderRecord
		deadline := time.Now().Add(30 * time.Second)
		backoff := 2 * time.Millisecond
		for {
			var err error
			claimed, err = r.ClaimRecharge(o.OrderNo, "test", true)
			if !errors.Is(err, ErrPaymentBusy) {
				if err != nil {
					return err
				}
				break
			}
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(backoff)
			if backoff < 100*time.Millisecond {
				backoff *= 2
			}
		}
		defer r.ReleaseRecharge(o.OrderNo, claimed.SyncVersion, 0, "")
		_, err := r.ApplyPaymentSnapshot(o.OrderNo, claimed.SyncVersion, s)
		return err
	}
	parallel := func(t *testing.T, n int, fn func(int) error) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; errs <- fn(i) }(i)
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
	}

	t.Run("64_concurrent_completions_credit_once", func(t *testing.T) {
		user := newUser("duplicate")
		o := newOrder("duplicate", user, 50)
		parallel(t, 64, func(int) error { return apply(o, snapshot(o)) })
		require.Equal(t, ToUnits(50), balance(user))
	})
	t.Run("32_orders_preserve_wallet_updates", func(t *testing.T) {
		user := newUser("independent")
		orders := make([]*RechargeOrderRecord, 32)
		for i := range orders {
			orders[i] = newOrder(fmt.Sprintf("independent-%d", i), user, 1.23)
		}
		parallel(t, 32, func(i int) error { return apply(orders[i], snapshot(orders[i])) })
		require.Equal(t, ToUnits(39.36), balance(user))
	})
	t.Run("expired_lease_cannot_overwrite_refund", func(t *testing.T) {
		user := newUser("fencing")
		o := newOrder("fencing", user, 50)
		first, err := r.ClaimRecharge(o.OrderNo, "test", true)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE recharge_orders SET lease_until=0 WHERE order_no=?`, o.OrderNo)
		require.NoError(t, err)
		second, err := r.ClaimRecharge(o.OrderNo, "test", true)
		require.NoError(t, err)
		current := snapshot(o)
		current.Adjustments = []PaymentAdjustment{{Kind: "refund", ID: "re_" + o.OrderNo, PaymentIntentID: current.PaymentIntentID, Amount: 5000, Currency: "cny", Status: "succeeded"}}
		_, err = r.ApplyPaymentSnapshot(o.OrderNo, second.SyncVersion, current)
		require.NoError(t, err)
		_, err = r.ApplyPaymentSnapshot(o.OrderNo, first.SyncVersion, snapshot(o))
		require.ErrorIs(t, err, ErrPaymentBusy)
		require.NoError(t, r.ReleaseRecharge(o.OrderNo, first.SyncVersion, 0, "stale"))
		fresh, err := r.GetRechargeOrder(o.OrderNo)
		require.NoError(t, err)
		require.NotZero(t, fresh.LeaseUntil)
		require.Equal(t, "refunded", fresh.Status)
		require.Zero(t, balance(user))
		require.NoError(t, r.ReleaseRecharge(o.OrderNo, second.SyncVersion, 0, ""))
	})
	t.Run("missing_wallet_rolls_back_payment", func(t *testing.T) {
		o := newOrder("missing-wallet", "nonexistent-"+suffix, 50)
		err := apply(o, snapshot(o))
		require.Error(t, err)
		fresh, err := r.GetRechargeOrder(o.OrderNo)
		require.NoError(t, err)
		require.Equal(t, "pending", fresh.Status)
		require.Zero(t, fresh.CreditedCents)
	})
	t.Run("idempotency_is_user_scoped_and_parameter_bound", func(t *testing.T) {
		user := newUser("intent")
		o := newOrder("intent", user, 50)
		same := *o
		same.OrderNo = "unused-number-" + suffix
		got, err := r.CreateRechargeIntent(&same)
		require.NoError(t, err)
		require.Equal(t, o.OrderNo, got.OrderNo)
		same.Amount = 51
		_, err = r.CreateRechargeIntent(&same)
		require.ErrorIs(t, err, ErrPaymentConflict)
		other := newUser("intent-other")
		same.Username = other
		same.OrderNo = "other-order-" + suffix
		_, err = r.CreateRechargeIntent(&same)
		require.NoError(t, err)
	})
	t.Run("event_deduplication_and_environment_binding", func(t *testing.T) {
		e := StripeEventRecord{ID: "evt_" + suffix, Type: "refund.updated", ObjectID: "re_" + suffix, Mode: "test"}
		done, err := r.SaveStripeEvent(e)
		require.NoError(t, err)
		require.False(t, done)
		require.NoError(t, r.FinishStripeEvent(e.ID, nil))
		done, err = r.SaveStripeEvent(e)
		require.NoError(t, err)
		require.True(t, done)
		e.ObjectID = "different-object"
		_, err = r.SaveStripeEvent(e)
		require.ErrorIs(t, err, ErrPaymentConflict)
		require.NoError(t, r.BindPaymentMode("test"))
		require.Error(t, r.BindPaymentMode("live"))
	})
	t.Run("redemption_concurrency", func(t *testing.T) {
		user := newUser("redemption")
		code := "PAYMENT-" + suffix
		require.NoError(t, r.CreateRedemptionCodes([]*RedemptionCodeRecord{{Code: code, Amount: 1.23}}))
		var mu sync.Mutex
		successes := 0
		parallel(t, 32, func(int) error {
			_, err := r.RedeemCode(code, user)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
			return nil
		})
		require.Equal(t, 1, successes)
		require.Equal(t, ToUnits(1.23), balance(user))
	})
}
