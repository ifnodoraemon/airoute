package storage

import (
	"context"
	"database/sql"
	"errors"
)

type billingAccount struct {
	key                           string
	userID, balance, budget, used int64
	exempt                        bool
	active                        bool
}

// Take the same key -> wallet locks as settlement. Redis and authentication
// snapshots supply no balance authority to this transaction.
func (r *Repository) lockBillingAccount(ctx context.Context, tx *Tx, key string) (billingAccount, error) {
	account := billingAccount{key: key, exempt: true}
	lock := ""
	if r.db.Dialect() == "postgres" {
		lock = " FOR UPDATE"
	}
	var status, tenant string
	query := `SELECT COALESCE(user_id, 0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), status, tenant_id FROM api_keys WHERE key = ?` + lock
	if err := tx.QueryRowContext(ctx, query, key).Scan(&account.userID, &account.budget, &account.used, &status, &tenant); err != nil {
		return account, err
	}
	account.active = status == "active" || status == ""
	if account.userID <= 0 {
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ? OR email = ? ORDER BY CASE WHEN username = ? THEN 0 ELSE 1 END, id LIMIT 1`, tenant, tenant, tenant).Scan(&account.userID)
		if errors.Is(err, sql.ErrNoRows) {
			return account, nil
		}
		if err != nil {
			return account, err
		}
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT balance_units, role, status FROM users WHERE id = ?`+lock, account.userID).Scan(&account.balance, &role, &status); err != nil {
		return account, err
	}
	account.active = account.active && status != "locked" && status != "deleted"
	account.exempt = role == "admin"
	return account, nil
}

func checkReservationHeadroom(ctx context.Context, tx *Tx, account billingAccount, rec *BillingReservation, maxInflight int) error {
	if !account.active {
		return ErrReservationAccount
	}
	if rec.AmountUnits < 0 {
		return ErrWalletReserved
	}
	if !account.exempt {
		var held, count int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_units), 0), COUNT(*) FROM billing_reservations WHERE user_id = ? AND id <> ?`, account.userID, rec.ID).Scan(&held, &count); err != nil {
			return err
		}
		if rec.AmountUnits > account.balance || held > account.balance-rec.AmountUnits || (maxInflight > 0 && count >= int64(maxInflight)) {
			return ErrWalletReserved
		}
	}
	if account.budget > 0 {
		var held int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_units), 0) FROM billing_reservations WHERE api_key = ? AND id <> ?`, account.key, rec.ID).Scan(&held); err != nil {
			return err
		}
		available := account.budget - account.used
		if available < rec.AmountUnits || held > available-rec.AmountUnits {
			return ErrKeyBudgetReserved
		}
	}
	return nil
}
