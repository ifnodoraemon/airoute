package storage

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/lib/pq"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// ErrSettlementNotInitialized is returned when the synchronous settlement
// runtime has not been installed (unit tests, CLI subcommands). Callers treat it
// as "no local settlement available" rather than as a billing failure: the
// asynchronous pipeline still applies the charge.
var ErrSettlementNotInitialized = errors.New("billing settlement is not initialized")

// settlementRepo is installed once at startup by InitBillingSettlement.
var settlementRepo *Repository

// InitBillingSettlement enables synchronous charge settlement for the data plane.
func InitBillingSettlement(repo *Repository) {
	settlementRepo = repo
}

// SettleChargeNow applies a charge's wallet movement immediately, without waiting
// for the audit log to be persisted.
//
// This is the second half of the billing/audit decoupling: the money side used to
// live inside the audit write transaction, so a healthy database but an unhealthy
// log pipeline meant a delayed charge. Now the data plane settles at request
// completion and the audit row follows asynchronously. If this call fails (for
// example the database is unreachable) the record still travels through
// Redis/spill and is settled later, so a failure here degrades to the previous
// behaviour instead of losing the charge.
func SettleChargeNow(ctx context.Context, rec *UsageLogRecord) error {
	repo := settlementRepo
	if repo == nil {
		return ErrSettlementNotInitialized
	}
	err := repo.SettleUsageCharge(ctx, rec)
	switch {
	case err == nil:
		telemetry.GlobalMetrics.BillingSettledSynchronously.Add(1)
	case errors.Is(err, ErrSettlementNotInitialized):
		// no settlement runtime installed; nothing to count
	default:
		countDeferredSettlement(err)
	}
	return err
}

// SettleUsageCharge applies one usage record's charge exactly once.
func (r *Repository) SettleUsageCharge(ctx context.Context, rec *UsageLogRecord) error {
	if rec == nil {
		return nil
	}
	normalizeUsageLog(rec)
	return r.settleCharges(ctx, []*UsageLogRecord{rec})
}

// settleCharges applies the money movement for a batch of usage records.
//
// A charge is applied only when its billing_id is newly inserted into
// billing_settlements, which makes this call idempotent across every path that
// can deliver a record (synchronous settlement, Redis redelivery, spill replay,
// local retry). Records already settled are marked so later calls skip them.
//
// Transient PostgreSQL lock conflicts (deadlock detected, lock_not_available from
// our 500ms lock_timeout, statement cancellation) are retried up to
// settlementMaxRetries times with a short jittered backoff: the work is short and
// idempotent, and a batch that loses the row-lock race would otherwise defer every
// charge in it. Anything else is returned immediately — retrying a real error only
// makes the failure slower.
func (r *Repository) settleCharges(ctx context.Context, logs []*UsageLogRecord) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = r.settleChargesOnce(ctx, logs)
		if err == nil {
			return nil
		}
		if !isRetryableSettlementErr(err) {
			return err
		}
		telemetry.GlobalMetrics.SettlementLockTimeouts.Add(1)
		if attempt >= settlementMaxRetries || ctx.Err() != nil {
			return err
		}
		if !sleepSettlementRetry(ctx, settlementRetryDelay(attempt)) {
			return err
		}
	}
}

// settlementMaxRetries is the number of extra attempts after the first one.
// Two retries at 20–80ms cover the common "another flush holds the wallet row"
// case without holding the caller for long: the request-side wait is much longer.
const settlementMaxRetries = 2

// settlementRetryBaseDelay / settlementRetryMaxDelay bound the jittered backoff.
// The jitter matters as much as the delay: two flushes that collided on a row must
// not wake up together and collide again.
const (
	settlementRetryBaseDelay = 20 * time.Millisecond
	settlementRetryMaxDelay  = 80 * time.Millisecond
)

// isRetryableSettlementErr reports whether a settlement failure is a transient
// PostgreSQL lock/statement conflict:
//
//	40P01 deadlock_detected        — a concurrent flush formed a lock cycle
//	55P03 lock_not_available       — our lock_timeout=500 expired on a hot row
//	57014 query_canceled           — statement_timeout/cancellation aborted the work
//
// SQLite has no per-statement lock deadline (busy_timeout handles contention), so
// its errors are never retried here. The check is a pure function over the error
// so it can be unit tested without a PostgreSQL server.
func isRetryableSettlementErr(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	switch string(pqErr.Code) {
	case "40P01", "55P03", "57014":
		return true
	}
	return false
}

// settlementRetryDelay returns the jittered backoff before retry number attempt
// (0-based): uniformly distributed in [base, max].
func settlementRetryDelay(attempt int) time.Duration {
	span := settlementRetryMaxDelay - settlementRetryBaseDelay
	if span <= 0 {
		return settlementRetryBaseDelay
	}
	return settlementRetryBaseDelay + time.Duration(rand.Int64N(int64(span)+1))
}

// sleepSettlementRetry waits for d unless the caller goes away first; it reports
// whether the wait completed.
func sleepSettlementRetry(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
