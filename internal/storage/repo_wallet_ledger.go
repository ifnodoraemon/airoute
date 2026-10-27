package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Wallet transaction types. opening is written by the migration for existing
// balances; the rest are written by the money-moving paths.
const (
	WalletTypeOpening    = "opening"
	WalletTypeRecharge   = "recharge"
	WalletTypeCharge     = "charge"
	WalletTypeRefund     = "refund"
	WalletTypeAdjustment = "adjustment"
	WalletTypeRedemption = "redemption"
)

// WalletTransactionRecord is one append-only wallet movement.
type WalletTransactionRecord struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	Type              string    `json:"type"`
	DeltaUnits        int64     `json:"delta_units"`
	BalanceAfterUnits int64     `json:"balance_after_units"`
	Operator          string    `json:"operator,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	RefID             string    `json:"ref_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func normalizeWalletTransaction(rec *WalletTransactionRecord) error {
	if rec == nil {
		return fmt.Errorf("wallet transaction is required")
	}
	rec.Type = strings.ToLower(strings.TrimSpace(rec.Type))
	switch rec.Type {
	case WalletTypeOpening, WalletTypeRecharge, WalletTypeCharge, WalletTypeRefund, WalletTypeAdjustment, WalletTypeRedemption:
	default:
		return fmt.Errorf("invalid wallet transaction type %q", rec.Type)
	}
	if rec.UserID <= 0 {
		return fmt.Errorf("wallet transaction user is required")
	}
	rec.Operator = strings.TrimSpace(rec.Operator)
	rec.Reason = strings.TrimSpace(rec.Reason)
	rec.RefID = strings.TrimSpace(rec.RefID)
	if len(rec.RefID) > 128 {
		rec.RefID = rec.RefID[:128]
	}
	return nil
}

const walletTransactionColumns = `id, user_id, type, delta_units, balance_after_units, operator, reason, ref_id, created_at`

// RecordWalletTransaction appends a movement inside an existing transaction. The
// boolean is false when a row with the same (user, type, ref) already exists, so
// retried charges and recharge webhooks are idempotent.
func (r *Repository) RecordWalletTransaction(tx *Tx, rec *WalletTransactionRecord) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("wallet transaction requires a database transaction")
	}
	if err := normalizeWalletTransaction(rec); err != nil {
		return false, err
	}
	result, err := tx.Exec(`INSERT INTO wallet_transactions
		(user_id, type, delta_units, balance_after_units, operator, reason, ref_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		rec.UserID, rec.Type, rec.DeltaUnits, rec.BalanceAfterUnits, rec.Operator, rec.Reason, rec.RefID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// RecordWalletTransactionContext is the standalone form for paths that do not
// already hold a transaction (recharge webhooks, manual adjustments).
func (r *Repository) RecordWalletTransactionContext(ctx context.Context, rec *WalletTransactionRecord) (bool, error) {
	if err := normalizeWalletTransaction(rec); err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO wallet_transactions
		(user_id, type, delta_units, balance_after_units, operator, reason, ref_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		rec.UserID, rec.Type, rec.DeltaUnits, rec.BalanceAfterUnits, rec.Operator, rec.Reason, rec.RefID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func scanWalletTransaction(row interface{ Scan(...any) error }) (*WalletTransactionRecord, error) {
	var rec WalletTransactionRecord
	if err := row.Scan(&rec.ID, &rec.UserID, &rec.Type, &rec.DeltaUnits, &rec.BalanceAfterUnits,
		&rec.Operator, &rec.Reason, &rec.RefID, &rec.CreatedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListWalletTransactions returns newest-first movements for one user.
func (r *Repository) ListWalletTransactions(userID int64, limit, offset int) ([]*WalletTransactionRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(`SELECT `+walletTransactionColumns+` FROM wallet_transactions
		WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*WalletTransactionRecord, 0)
	for rows.Next() {
		rec, err := scanWalletTransaction(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, rows.Err()
}

// WalletLedgerSumContext returns the sum of all movements. It should equal the
// user's balance_units once opening is present; the verification job uses it to
// detect drift.
func (r *Repository) WalletLedgerSumContext(ctx context.Context, userID int64) (int64, error) {
	var sum int64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(delta_units), 0) FROM wallet_transactions WHERE user_id = ?`, userID).Scan(&sum)
	return sum, err
}

// LatestWalletBalanceContext returns the last recorded balance_after_units.
func (r *Repository) LatestWalletBalanceContext(ctx context.Context, userID int64) (int64, error) {
	var balance int64
	err := r.db.QueryRowContext(ctx, `SELECT balance_after_units FROM wallet_transactions WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return balance, err
}
