package storage

import (
	"context"
	"errors"
)

var (
	ErrWalletReserved     = errors.New("wallet balance cannot cover the request reservation")
	ErrKeyBudgetReserved  = errors.New("api key budget cannot cover the request reservation")
	ErrReservationMissing = errors.New("billing reservation is no longer available")
	ErrReservationAccount = errors.New("billing account is unavailable or inactive")
)

// BillingReservation identifies one request's durable pre-authorisation. Amounts
// are integer currency units; cancellation and settlement always use its ID.
type BillingReservation struct {
	ID          string
	APIKey      string
	UserID      int64
	AmountUnits int64
}

func (r *Repository) ReserveBilling(ctx context.Context, key string, units int64, maxInflight int) (*BillingReservation, error) {
	reservation := &BillingReservation{ID: NewBillingID(), APIKey: NormalizeStoredAPIKeyReference(key), AmountUnits: units}
	exempt := false
	err := r.reservationTransaction(ctx, func(tx *Tx) error {
		account, err := r.lockBillingAccount(ctx, tx, reservation.APIKey)
		if err != nil {
			return err
		}
		if !account.active {
			return ErrReservationAccount
		}
		if account.exempt && account.budget <= 0 {
			exempt = true
			return nil
		}
		reservation.UserID = account.userID
		if err := checkReservationHeadroom(ctx, tx, account, reservation, maxInflight); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO billing_reservations (id, api_key, user_id, amount_units) VALUES (?, ?, ?, ?)`,
			reservation.ID, reservation.APIKey, reservation.UserID, units)
		return err
	})
	if exempt {
		return nil, err
	}
	return reservation, err
}

// ResizeBilling requires the full requested amount; partial reservations never
// authorise upstream work. The fresh wallet/key rows and the holds share a lock.
func (r *Repository) ResizeBilling(ctx context.Context, id string, units int64) error {
	return r.mutateBillingReservation(ctx, id, func(tx *Tx, account billingAccount, rec *BillingReservation) error {
		if units <= rec.AmountUnits {
			return nil
		}
		rec.AmountUnits = units
		if err := checkReservationHeadroom(ctx, tx, account, rec, 0); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE billing_reservations SET amount_units = ? WHERE id = ?`, units, id)
		return err
	})
}

// PendBilling retains at least the amount already delivered. Failure is safe:
// the original durable hold remains, and the audit pipeline carries its ID.
func (r *Repository) PendBilling(ctx context.Context, id, billingID string, units int64) error {
	return r.mutateBillingReservation(ctx, id, func(tx *Tx, _ billingAccount, rec *BillingReservation) error {
		if units < rec.AmountUnits {
			units = rec.AmountUnits
		}
		_, err := tx.ExecContext(ctx, `UPDATE billing_reservations SET amount_units = ?, pending = TRUE, billing_id = ? WHERE id = ?`, units, billingID, id)
		return err
	})
}

// CancelBilling releases only the named request, and refuses to erase a pending
// charge. A repeated cancellation cannot affect a different request.
func (r *Repository) CancelBilling(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM billing_reservations WHERE id = ? AND pending = FALSE`, id)
	return err
}

func (r *Repository) mutateBillingReservation(ctx context.Context, id string, change func(*Tx, billingAccount, *BillingReservation) error) error {
	return r.reservationTransaction(ctx, func(tx *Tx) error {
		rec := &BillingReservation{ID: id}
		if err := tx.QueryRowContext(ctx, `SELECT api_key FROM billing_reservations WHERE id = ?`, id).Scan(&rec.APIKey); err != nil {
			return ErrReservationMissing
		}
		account, err := r.lockBillingAccount(ctx, tx, rec.APIKey)
		if err != nil {
			return err
		}
		rec.UserID = account.userID
		if err := tx.QueryRowContext(ctx, `SELECT amount_units FROM billing_reservations WHERE id = ?`, id).Scan(&rec.AmountUnits); err != nil {
			return ErrReservationMissing
		}
		return change(tx, account, rec)
	})
}

func (r *Repository) reservationTransaction(ctx context.Context, change func(*Tx) error) error {
	for attempt := 0; ; attempt++ {
		tx, err := r.db.BeginTx(ctx)
		if err != nil {
			return err
		}
		err = change(tx)
		if err == nil {
			err = tx.Commit()
		}
		_ = tx.Rollback()
		if !isRetryableSettlementErr(err) || attempt >= settlementMaxRetries || !sleepSettlementRetry(ctx, settlementRetryDelay(attempt)) {
			return err
		}
	}
}
