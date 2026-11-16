package storage

import (
	"context"
	"encoding/json"
	"time"
)

// ReleaseBillingHold frees a reservation by ID regardless of its pending flag.
// Settlement already deletes the hold, so releasing an absent or settled hold is
// a no-op. Unlike CancelBilling (which refuses pending charges) this also clears
// a pending hold, which is required when an asynchronous job is abandoned —
// expired or deleted — before it ever reaches settlement.
func (r *Repository) ReleaseBillingHold(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM billing_reservations WHERE id = ?`, id)
	return err
}

func releaseBillingHoldTx(ctx context.Context, tx *Tx, id string) error {
	if id == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM billing_reservations WHERE id = ?`, id)
	return err
}

// reservationIDFromBilling extracts the hold ID stored in a native job's billing
// snapshot. It returns empty when the job carried no reservation (free/exempt
// requests), so callers can skip the release.
func reservationIDFromBilling(billing string) string {
	var snap struct {
		ReservationID string `json:"_airoute_reservation_id"`
	}
	if json.Unmarshal([]byte(billing), &snap) != nil {
		return ""
	}
	return snap.ReservationID
}

// ReapOrphanBillingHolds frees pending holds older than grace that belong to no
// live asynchronous job and no synchronous charge. A synchronous hold always has
// a billing_usage_journal row (PendBillingRecord writes it atomically), so the
// journal NOT EXISTS excludes them; a running native job still has its state, so
// the native_states NOT EXISTS excludes those too. What remains is an orphaned
// native-job hold — e.g. a crash between Hold and SaveNativeState — which would
// otherwise reserve balance forever. The grace covers the Hold→Save window.
func (r *Repository) ReapOrphanBillingHolds(ctx context.Context, grace time.Duration) (int64, error) {
	cutoff := r.dialectTime(time.Now().Add(-grace))
	res, err := r.db.ExecContext(ctx, `DELETE FROM billing_reservations
		WHERE pending = TRUE AND created_at < ?
		AND NOT EXISTS (SELECT 1 FROM billing_usage_journal j WHERE j.billing_id = billing_reservations.billing_id)
		AND NOT EXISTS (SELECT 1 FROM native_states s WHERE s.billing LIKE '%' || billing_reservations.id || '%')`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}