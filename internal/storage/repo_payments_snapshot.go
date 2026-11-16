package storage

import (
	"fmt"
	"time"
)

// ApplyPaymentSnapshot reconciles a Stripe checkout snapshot onto the order and
// the wallet under a row lock. Extracted from repo_payments.go to keep the order
// repository under the size red line.
func (r *Repository) ApplyPaymentSnapshot(number string, version int64, s PaymentSnapshot) (*RechargeOrderRecord, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := `SELECT ` + rechargeColumns + ` FROM recharge_orders WHERE order_no = ?`
	if r.db.Dialect() == "postgres" {
		query += ` FOR UPDATE`
	}
	o, err := scanRecharge(tx.QueryRow(query, number))
	if err != nil {
		return nil, err
	}
	if o.SyncVersion != version || o.LeaseUntil <= time.Now().Unix() {
		return nil, ErrPaymentBusy
	}
	if o.Channel != "stripe" || o.IdempotencyKey == "" || s.SessionID == "" || (o.StripeSessionID != "" && o.StripeSessionID != s.SessionID) || o.PaymentMode != s.Mode || s.Currency != "CNY" || o.Currency != s.Currency || o.Cents() != s.AmountCents || (o.PaymentIntentID != "" && o.PaymentIntentID != s.PaymentIntentID) {
		return nil, ErrPaymentConflict
	}
	if s.Status != "pending" && s.Status != "paid" && s.Status != "expired" && s.Status != "failed" {
		return nil, ErrPaymentConflict
	}
	if s.Status == "paid" && s.PaymentIntentID == "" {
		return nil, ErrPaymentConflict
	}
	for _, a := range s.Adjustments {
		if a.ID == "" || a.PaymentIntentID != s.PaymentIntentID || a.LiveMode != (s.Mode == "live") || len(a.Currency) != 3 || a.Amount < 1 || a.Amount > int64(MaxUserBalance*100) {
			return nil, ErrPaymentConflict
		}
		walletCents := min(a.Amount, o.Cents())
		valid := false
		if a.Kind == "refund" {
			if a.Currency != "cny" || a.Amount > o.Cents() {
				return nil, ErrPaymentConflict
			}
			switch a.Status {
			case "pending", "requires_action", "succeeded", "failed", "canceled":
				valid = true
			}
		}
		if a.Kind == "dispute" {
			// FX can make a dispute exceed the charge or use another currency.
			// Reserve the original principal when an exact CNY amount is unknown;
			// keep the raw amount/currency for review rather than invent an FX rate.
			if a.Currency != "cny" {
				walletCents = o.Cents()
			}
			switch a.Status {
			case "needs_response", "under_review", "appeal_needs_response", "appeal_under_review", "lost", "won", "prevented", "warning_needs_response", "warning_under_review", "warning_closed":
				valid = true
			}
		}
		if !valid {
			return nil, ErrPaymentConflict
		}
		res, err := tx.Exec(`INSERT INTO payment_adjustments(kind,object_id,order_no,amount_minor,currency,wallet_cents,status) VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(kind,object_id) DO UPDATE SET status=excluded.status, amount_minor=excluded.amount_minor,currency=excluded.currency,wallet_cents=excluded.wallet_cents,updated_at=CURRENT_TIMESTAMP
			WHERE payment_adjustments.order_no=excluded.order_no AND (excluded.kind='dispute' OR (payment_adjustments.amount_minor=excluded.amount_minor AND payment_adjustments.currency=excluded.currency))`, a.Kind, a.ID, number, a.Amount, a.Currency, walletCents, a.Status)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, ErrPaymentConflict
		}
	}
	var refunded, held, disputed int64
	err = tx.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN kind='refund' AND status='succeeded' THEN wallet_cents ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN kind='refund' AND status IN ('pending','requires_action') THEN wallet_cents ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN kind='dispute' AND status NOT IN ('won','warning_closed','prevented') THEN wallet_cents ELSE 0 END),0)
		FROM payment_adjustments WHERE order_no=?`, number).Scan(&refunded, &held, &disputed)
	if err != nil {
		return nil, err
	}
	if refunded+held > o.Cents() {
		return nil, ErrPaymentConflict
	}
	credited := o.CreditedCents
	if s.Status == "paid" {
		credited = o.Cents()
	}
	reversed := int64(0)
	if credited > 0 {
		reversed = min(credited, refunded+held+disputed)
	}
	deltaCents := (credited - o.CreditedCents) - (reversed - o.ReversedCents)
	if deltaCents != 0 {
		units := deltaCents * (UnitsPerCurrency / 100)
		limit := ToUnits(MaxUserBalance)
		res, err := tx.Exec(`UPDATE users SET balance_units=COALESCE(balance_units,0)+?, updated_at=CURRENT_TIMESTAMP
			WHERE username=? AND COALESCE(balance_units,0)+? BETWEEN ? AND ?`, units, o.Username, units, -limit, limit)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, fmt.Errorf("wallet unavailable or balance outside allowed range")
		}
	}
	status := s.Status
	if credited > 0 {
		switch {
		case disputed > 0:
			status = "disputed"
		case held > 0:
			status = "refund_pending"
		case refunded == credited:
			status = "refunded"
		case refunded > 0:
			status = "partially_refunded"
		default:
			status = "paid"
		}
	} else if (o.Status == "failed" || o.Status == "expired") && status == "pending" {
		status = o.Status
	}
	_, err = tx.Exec(`UPDATE recharge_orders SET stripe_session_id=?, payment_intent_id=?, checkout_url=?, expires_at=?, status=?, credited_cents=?, reversed_cents=?, refunded_cents=?, refund_pending_cents=?, disputed_cents=?, updated_at=CURRENT_TIMESTAMP WHERE order_no=?`, s.SessionID, s.PaymentIntentID, s.CheckoutURL, s.ExpiresAt, status, credited, reversed, refunded, held, disputed, number)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetRechargeOrder(number)
}
