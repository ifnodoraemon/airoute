package storage

import (
	"database/sql"
	"fmt"
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
	if rec.Status == "" {
		rec.Status = "active"
	}
	id, err := r.db.InsertGetID(`INSERT INTO redemption_codes (code, name, amount, status, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Code, rec.Name, rec.Amount, rec.Status)
	if err != nil {
		// Table doesn't have updated_at, handle gracefully
		id, err = r.db.InsertGetID(`INSERT INTO redemption_codes (code, name, amount, status) VALUES (?, ?, ?, ?)`,
			rec.Code, rec.Name, rec.Amount, rec.Status)
		if err != nil {
			return err
		}
	}
	rec.ID = id
	return nil
}

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

	if rec.Status != "active" {
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
	_, err = tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`), rec.Amount, username)
	if err != nil {
		return nil, fmt.Errorf("充值到账户余额失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	rec.Status = "used"
	rec.UsedBy = username
	rec.UsedAt = &now
	return &rec, nil
}

// DeleteRedemptionCode removes a redemption code.
func (r *Repository) DeleteRedemptionCode(id int64) error {
	_, err := r.db.Exec(`DELETE FROM redemption_codes WHERE id = ?`, id)
	return err
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
	row := tx.QueryRow(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE order_no = ?`, orderNo)
	if err := row.Scan(&rec.ID, &rec.OrderNo, &rec.Username, &rec.Amount, &rec.Currency, &rec.Channel, &rec.StripeSessionID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, fmt.Errorf("订单不存在: %w", err)
	}

	if rec.Status == "paid" {
		return &rec, nil // Already completed (idempotent)
	}

	_, err = tx.Exec(`UPDATE recharge_orders SET status = 'paid', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, rec.ID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, rec.Amount, rec.Username)
	if err != nil {
		return nil, err
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
		rows, err = r.db.Query(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE username = ? ORDER BY id DESC LIMIT 50`, username)
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
	return list, nil
}

