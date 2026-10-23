package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// RedemptionCodeRecord represents a gift or balance redemption code.
type RedemptionCodeRecord struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Amount    float64    `json:"amount"`
	Status    string     `json:"status"` // "active", "used"
	UsedBy    string     `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateRedemptionCode creates a new redemption card.
func (r *Repository) CreateRedemptionCode(rec *RedemptionCodeRecord) error {
	return r.CreateRedemptionCodes([]*RedemptionCodeRecord{rec})
}

// CreateRedemptionCodes commits the entire batch or none of it, so an operator
// can safely retry a failed generation without leaving undistributed cards.
func (r *Repository) CreateRedemptionCodes(records []*RedemptionCodeRecord) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, rec := range records {
		if err := insertRedemptionCode(tx, rec); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertRedemptionCode(db recordInserter, rec *RedemptionCodeRecord) error {
	if rec == nil || !ValidRechargeAmount(rec.Amount) {
		return fmt.Errorf("兑换金额须为 0.01 至 999999.99 元，最多保留两位小数")
	}
	if rec.Status == "" {
		rec.Status = "active"
	}
	id, err := db.InsertGetID(`INSERT INTO redemption_codes (code, name, amount, status) VALUES (?, ?, ?, ?)`,
		rec.Code, rec.Name, rec.Amount, rec.Status)
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}

func ValidRechargeAmount(amount float64) bool {
	cents := math.Round(amount * 100)
	return !math.IsNaN(cents) && !math.IsInf(cents, 0) && cents >= 1 && cents <= 99_999_999 && math.Abs(amount*100-cents) <= 0.000001
}

var ErrRedemptionUsed = errors.New("已兑换的记录须保留，不能删除")

// ListRedemptionCodes returns all redemption codes.
func (r *Repository) ListRedemptionCodes() ([]*RedemptionCodeRecord, error) {
	rows, err := r.db.Query(`SELECT id, code, COALESCE(name, ''), amount, COALESCE(status, 'active'), COALESCE(used_by, ''), used_at, created_at FROM redemption_codes ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*RedemptionCodeRecord
	for rows.Next() {
		var rec RedemptionCodeRecord
		var usedAt sql.NullTime
		if err := rows.Scan(&rec.ID, &rec.Code, &rec.Name, &rec.Amount, &rec.Status, &rec.UsedBy, &usedAt, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if usedAt.Valid {
			t := usedAt.Time
			rec.UsedAt = &t
		}
		list = append(list, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// RedeemCode redeems a code and adds its amount to the specified user's balance.
func (r *Repository) RedeemCode(code, username string) (*RedemptionCodeRecord, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("兑换码不能为空")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var rec RedemptionCodeRecord
	var usedAt sql.NullTime
	row := tx.QueryRow(r.db.Rebind(`SELECT id, code, COALESCE(name, ''), amount, COALESCE(status, 'active'), COALESCE(used_by, ''), used_at, created_at FROM redemption_codes WHERE code = ?`), code)
	if err := row.Scan(&rec.ID, &rec.Code, &rec.Name, &rec.Amount, &rec.Status, &rec.UsedBy, &usedAt, &rec.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("无效的兑换码")
		}
		return nil, err
	}

	if rec.Status != "active" || !ValidRechargeAmount(rec.Amount) {
		return nil, fmt.Errorf("该兑换码已被使用或已失效")
	}

	// Mark as used with atomic status check to prevent race-condition double redemption
	now := time.Now()
	res, err := tx.Exec(r.db.Rebind(`UPDATE redemption_codes SET status = 'used', used_by = ?, used_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'active'`), username, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("更新兑换状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return nil, fmt.Errorf("该兑换码已被使用或已失效")
	}

	// Credit user balance
	credited, err := tx.Exec(`UPDATE users SET balance = balance + ?, balance_units = COALESCE(balance_units, 0) + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ? AND status = 'active'`, rec.Amount, ToUnits(rec.Amount), username)
	if err != nil {
		return nil, fmt.Errorf("充值到账户余额失败: %w", err)
	}
	if err := requireUserUpdated(credited, nil); err != nil {
		return nil, fmt.Errorf("充值失败: 用户不存在或不可用 (%s): %w", username, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	rec.Status = "used"
	rec.UsedBy = username
	rec.UsedAt = &now
	return &rec, nil
}

// DeleteRedemptionCode removes unused codes while preserving redemption evidence.
func (r *Repository) DeleteRedemptionCode(id int64) error {
	err := requireUserUpdated(r.db.Exec(`DELETE FROM redemption_codes WHERE id = ? AND status = 'active'`, id))
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var status string
	if err := r.db.QueryRow(`SELECT status FROM redemption_codes WHERE id = ?`, id).Scan(&status); err != nil {
		return err
	}
	return ErrRedemptionUsed
}

// RechargeOrderRecord represents a user wallet top-up order.
type RechargeOrderRecord struct {
	ID              int64     `json:"id"`
	OrderNo         string    `json:"order_no"`
	Username        string    `json:"username"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Channel         string    `json:"channel"`           // "stripe", "sandbox"
	StripeSessionID string    `json:"stripe_session_id"` // Stripe Checkout Session ID
	Status          string    `json:"status"`            // "pending", "paid", "cancelled"
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateRechargeOrder inserts a new recharge order.
func (r *Repository) CreateRechargeOrder(rec *RechargeOrderRecord) error {
	if rec.Currency == "" {
		rec.Currency = "CNY"
	}
	if rec.Status == "" {
		rec.Status = "pending"
	}
	id, err := r.db.InsertGetID(`INSERT INTO recharge_orders (order_no, username, amount, currency, channel, stripe_session_id, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.OrderNo, rec.Username, rec.Amount, rec.Currency, rec.Channel, rec.StripeSessionID, rec.Status)
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}

// GetRechargeOrder returns a checkout record, including its verified payment status.
func (r *Repository) GetRechargeOrder(orderNo string) (*RechargeOrderRecord, error) {
	var rec RechargeOrderRecord
	err := r.db.QueryRow(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE order_no = ?`, orderNo).
		Scan(&rec.ID, &rec.OrderNo, &rec.Username, &rec.Amount, &rec.Currency, &rec.Channel, &rec.StripeSessionID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// CloseRechargeOrder records a terminal checkout event without overwriting a
// confirmed payment, including when Stripe delivers events out of order.
func (r *Repository) CloseRechargeOrder(orderNo, status string) error {
	if status != "expired" && status != "failed" {
		return fmt.Errorf("invalid terminal recharge status: %s", status)
	}
	_, err := r.db.Exec(`UPDATE recharge_orders SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE order_no = ? AND status = 'pending'`, status, orderNo)
	return err
}

// CompleteRechargeOrder marks an order as paid and credits the user's wallet balance.
func (r *Repository) CompleteRechargeOrder(orderNo string) (*RechargeOrderRecord, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var rec RechargeOrderRecord
	row := tx.QueryRow(r.db.Rebind(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE order_no = ?`), orderNo)
	if err := row.Scan(&rec.ID, &rec.OrderNo, &rec.Username, &rec.Amount, &rec.Currency, &rec.Channel, &rec.StripeSessionID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, fmt.Errorf("订单不存在: %w", err)
	}

	if rec.Status == "paid" {
		return &rec, nil // Already completed (idempotent)
	}

	// Atomic state transition: only the transaction that flips pending -> paid
	// credits the wallet, so concurrent webhook retries cannot double-credit.
	res, err := tx.Exec(r.db.Rebind(`UPDATE recharge_orders SET status = 'paid', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'paid'`), rec.ID)
	if err != nil {
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		rec.Status = "paid"
		return &rec, nil
	}

	credit, err := tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance + ?, balance_units = COALESCE(balance_units, 0) + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`), rec.Amount, ToUnits(rec.Amount), rec.Username)
	if err != nil {
		return nil, err
	}
	if affected, _ := credit.RowsAffected(); affected == 0 {
		return nil, fmt.Errorf("充值失败: 用户不存在 (%s)", rec.Username)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	rec.Status = "paid"
	return &rec, nil
}

// ListRechargeOrders returns recharge orders for a specific user or all users if username is empty.
func (r *Repository) ListRechargeOrders(username string) ([]*RechargeOrderRecord, error) {
	var rows *sql.Rows
	var err error
	if username != "" {
		rows, err = r.db.Query(r.db.Rebind(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE username = ? ORDER BY id DESC LIMIT 50`), username)
	} else {
		rows, err = r.db.Query(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders ORDER BY id DESC LIMIT 100`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*RechargeOrderRecord
	for rows.Next() {
		var rec RechargeOrderRecord
		if err := rows.Scan(&rec.ID, &rec.OrderNo, &rec.Username, &rec.Amount, &rec.Currency, &rec.Channel, &rec.StripeSessionID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}
