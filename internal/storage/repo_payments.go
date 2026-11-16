package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrPaymentConflict = errors.New("payment intent conflicts with existing parameters")
	ErrPaymentBusy     = errors.New("payment verification already in progress")
)

type RechargeOrderRecord struct {
	ID                 int64     `json:"id"`
	OrderNo            string    `json:"order_no"`
	Username           string    `json:"username"`
	Amount             float64   `json:"amount"`
	Currency           string    `json:"currency"`
	Channel            string    `json:"channel"`
	StripeSessionID    string    `json:"stripe_session_id"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	IdempotencyKey     string    `json:"-"`
	PaymentMode        string    `json:"payment_mode"`
	CheckoutURL        string    `json:"-"`
	SuccessURL         string    `json:"-"`
	CancelURL          string    `json:"-"`
	ExpiresAt          int64     `json:"expires_at"`
	PaymentIntentID    string    `json:"-"`
	CreditedCents      int64     `json:"credited_cents"`
	ReversedCents      int64     `json:"reversed_cents"`
	RefundedCents      int64     `json:"refunded_cents"`
	RefundPendingCents int64     `json:"refund_pending_cents"`
	DisputedCents      int64     `json:"disputed_cents"`
	NextReconcileAt    int64     `json:"-"`
	SyncVersion        int64     `json:"-"`
	LeaseUntil         int64     `json:"-"`
	LastSyncError      string    `json:"-"`
}

func (o *RechargeOrderRecord) Cents() int64 { return int64(math.Round(o.Amount * 100)) }

const rechargeColumns = `id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at,
 idempotency_key, payment_mode, checkout_url, success_url, cancel_url, expires_at, payment_intent_id,
 credited_cents, reversed_cents, refunded_cents, refund_pending_cents, disputed_cents,
 next_reconcile_at, sync_version, lease_until, last_sync_error`

type paymentScanner interface{ Scan(...any) error }

func scanRecharge(row paymentScanner) (*RechargeOrderRecord, error) {
	o := &RechargeOrderRecord{}
	err := row.Scan(&o.ID, &o.OrderNo, &o.Username, &o.Amount, &o.Currency, &o.Channel, &o.StripeSessionID, &o.Status, &o.CreatedAt, &o.UpdatedAt,
		&o.IdempotencyKey, &o.PaymentMode, &o.CheckoutURL, &o.SuccessURL, &o.CancelURL, &o.ExpiresAt, &o.PaymentIntentID,
		&o.CreditedCents, &o.ReversedCents, &o.RefundedCents, &o.RefundPendingCents, &o.DisputedCents,
		&o.NextReconcileAt, &o.SyncVersion, &o.LeaseUntil, &o.LastSyncError)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (r *Repository) BindPaymentMode(mode string) error {
	if mode != "live" && mode != "test" {
		return ErrPaymentConflict
	}
	bound, err := r.GetOrCreateSetting("stripe_payment_mode", mode)
	if err != nil {
		return err
	}
	if bound != mode {
		return fmt.Errorf("database is bound to Stripe %s mode; use a separate database for %s", bound, mode)
	}
	return nil
}

func (r *Repository) CheckPaymentMode(mode string) error {
	var bound string
	err := r.db.QueryRow(`SELECT value FROM system_settings WHERE key='stripe_payment_mode'`).Scan(&bound)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if bound != mode {
		return fmt.Errorf("database is bound to Stripe %s mode; use a separate database for %s", bound, mode)
	}
	return nil
}

// CreateRechargeIntent persists the business intent BEFORE contacting Stripe.
// A repeated key is usable only by the same user, mode, amount and currency.
func (r *Repository) CreateRechargeIntent(o *RechargeOrderRecord) (*RechargeOrderRecord, error) {
	if o == nil || !ValidRechargeAmount(o.Amount) || o.Currency != "CNY" || o.IdempotencyKey == "" || o.OrderNo == "" || o.Username == "" || (o.PaymentMode != "live" && o.PaymentMode != "test") {
		return nil, ErrPaymentConflict
	}
	_, err := r.db.Exec(`INSERT INTO recharge_orders
		(order_no, username, amount, currency, channel, status, idempotency_key, payment_mode, success_url, cancel_url, expires_at, next_reconcile_at)
		VALUES (?, ?, ?, ?, 'stripe', 'pending', ?, ?, ?, ?, ?, 0)
		ON CONFLICT (username, idempotency_key) WHERE idempotency_key <> '' DO NOTHING`,
		o.OrderNo, o.Username, o.Amount, o.Currency, o.IdempotencyKey, o.PaymentMode, o.SuccessURL, o.CancelURL, o.ExpiresAt)
	if err != nil {
		return nil, err
	}
	stored, err := scanRecharge(r.db.QueryRow(`SELECT `+rechargeColumns+` FROM recharge_orders WHERE username = ? AND idempotency_key = ?`, o.Username, o.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	if stored.Cents() != o.Cents() || stored.Currency != o.Currency || stored.PaymentMode != o.PaymentMode {
		return nil, ErrPaymentConflict
	}
	return stored, nil
}

func (r *Repository) GetRechargeOrder(number string) (*RechargeOrderRecord, error) {
	return scanRecharge(r.db.QueryRow(`SELECT `+rechargeColumns+` FROM recharge_orders WHERE order_no = ?`, number))
}

func (r *Repository) FindRechargeByPaymentIntent(id string) (*RechargeOrderRecord, error) {
	return scanRecharge(r.db.QueryRow(`SELECT `+rechargeColumns+` FROM recharge_orders WHERE payment_intent_id = ? AND payment_intent_id <> ''`, id))
}

func (r *Repository) ListRechargeOrders(username string) ([]*RechargeOrderRecord, error) {
	query, args := `SELECT `+rechargeColumns+` FROM recharge_orders`, []any{}
	if username != "" {
		query += ` WHERE username = ?`
		args = append(args, username)
	}
	query += ` ORDER BY id DESC LIMIT 100`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []*RechargeOrderRecord{}
	for rows.Next() {
		o, err := scanRecharge(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

// The lease spans the API reads as well as the commit. A fencing version rejects
// a response that arrives after another replica has taken an expired lease.
func (r *Repository) ClaimRecharge(number, mode string, force bool) (*RechargeOrderRecord, error) {
	now := time.Now().Unix()
	query := `UPDATE recharge_orders SET sync_version = sync_version + 1, lease_until = ? WHERE order_no = ? AND payment_mode = ? AND idempotency_key <> '' AND lease_until <= ?`
	args := []any{now + 45, number, mode, now}
	if !force {
		query += ` AND next_reconcile_at <= ?`
		args = append(args, now)
	}
	var version int64
	err := r.db.QueryRow(query+` RETURNING sync_version`, args...).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPaymentBusy
	}
	if err != nil {
		return nil, err
	}
	o, err := r.GetRechargeOrder(number)
	if err != nil {
		return nil, err
	}
	if o.SyncVersion != version {
		return nil, ErrPaymentBusy
	}
	return o, nil
}

func (r *Repository) ReleaseRecharge(number string, version, next int64, message string) error {
	if len(message) > 256 {
		message = message[:256]
	}
	_, err := r.db.Exec(`UPDATE recharge_orders SET lease_until = 0, next_reconcile_at = ?, last_sync_error = ? WHERE order_no = ? AND sync_version = ?`, next, message, number, version)
	return err
}

func (r *Repository) DueRechargeOrders(mode string, limit int) ([]string, error) {
	rows, err := r.db.Query(`SELECT order_no FROM recharge_orders WHERE payment_mode = ? AND idempotency_key <> '' AND next_reconcile_at <= ? AND lease_until <= ? AND status NOT IN ('expired','failed') ORDER BY next_reconcile_at, id LIMIT ?`, mode, time.Now().Unix(), time.Now().Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []string
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		list = append(list, number)
	}
	return list, rows.Err()
}

type PaymentAdjustment struct {
	Kind            string
	ID              string `json:"id"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	PaymentIntentID string `json:"payment_intent"`
	LiveMode        bool   `json:"-"`
}

type PaymentSnapshot struct {
	SessionID, PaymentIntentID, Mode, Currency, Status, CheckoutURL string
	AmountCents, ExpiresAt                                          int64
	Adjustments                                                     []PaymentAdjustment
}

// ApplyPaymentSnapshot atomically moves the wallet to the verified NET amount.
// Pending refunds and open/lost disputes reserve their amount; failed refunds
// and won/closed inquiries release it. An already spent credit becomes debt.
type StripeEventRecord struct{ ID, Type, ObjectID, Mode string }

func (r *Repository) SaveStripeEvent(e StripeEventRecord) (bool, error) {
	_, err := r.db.Exec(`INSERT INTO stripe_events(id,event_type,object_id,payment_mode) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING`, e.ID, e.Type, e.ObjectID, e.Mode)
	if err != nil {
		return false, err
	}
	var kind, object, mode string
	var processed int
	err = r.db.QueryRow(`SELECT event_type,object_id,payment_mode,processed FROM stripe_events WHERE id=?`, e.ID).Scan(&kind, &object, &mode, &processed)
	if err != nil {
		return false, err
	}
	if kind != e.Type || object != e.ObjectID || mode != e.Mode {
		return false, ErrPaymentConflict
	}
	return processed != 0, nil
}

func (r *Repository) FinishStripeEvent(id string, processingError error) error {
	message := ""
	done := 1
	if processingError != nil {
		message = processingError.Error()
		done = 0
	}
	if len(message) > 256 {
		message = message[:256]
	}
	_, err := r.db.Exec(`UPDATE stripe_events SET processed=?, attempts=attempts+1, next_attempt_at=?, last_error=? WHERE id=? AND processed=0`, done, time.Now().Add(time.Minute).Unix(), message, id)
	return err
}

func (r *Repository) PendingStripeEvents(mode string, limit int) ([]StripeEventRecord, error) {
	rows, err := r.db.Query(`SELECT id,event_type,object_id,payment_mode FROM stripe_events WHERE payment_mode=? AND processed=0 AND next_attempt_at<=? ORDER BY created_at,id LIMIT ?`, mode, time.Now().Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []StripeEventRecord
	for rows.Next() {
		var e StripeEventRecord
		if err := rows.Scan(&e.ID, &e.Type, &e.ObjectID, &e.Mode); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
