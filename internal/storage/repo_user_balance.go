package storage

import (
	"database/sql"
	"errors"
	"math"
)

const MaxUserBalance = 1_000_000_000.0

var ErrBalanceOutOfRange = errors.New("账户余额或调整金额必须在 -1000000000 至 1000000000 元之间")

func validUserBalance(amount float64) bool {
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && math.Abs(amount) <= MaxUserBalance
}

func requireUserUpdated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

// UpdateUserBalance adjusts a user's wallet balance by a delta amount.
//
// The update is atomic: it must not read the balance, compute a new value and
// write it back, or a concurrent settlement deduction would be lost. RETURNING
// gives the ledger the resulting balance without a second read.
func (r *Repository) UpdateUserBalance(username string, delta float64) error {
	if !validUserBalance(delta) {
		return ErrBalanceOutOfRange
	}
	units := ToUnits(delta)
	limit := ToUnits(MaxUserBalance)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	var status string
	if err := tx.QueryRow(`SELECT id, COALESCE(status, 'active') FROM users WHERE username = ?`, username).Scan(&userID, &status); err != nil {
		return err
	}
	if status == "deleted" {
		return ErrBalanceOutOfRange
	}

	var newUnits int64
	err = tx.QueryRow(`UPDATE users SET
			balance_units = COALESCE(balance_units, 0) + ?,
			balance = (COALESCE(balance_units, 0) + ?) * 1.0 / 1000000000.0,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status <> 'deleted'
			AND COALESCE(balance_units, 0) + ? BETWEEN ? AND ?
		RETURNING balance_units`,
		units, units, userID, units, -limit, limit).Scan(&newUnits)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBalanceOutOfRange
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SetUserBalance sets an exact wallet balance for a user.
func (r *Repository) SetUserBalance(username string, balance float64) error {
	if !validUserBalance(balance) {
		return ErrBalanceOutOfRange
	}
	newUnits := ToUnits(balance)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	var status string
	// The old code ran a separate SELECT ... FOR UPDATE (lockWalletUserRows) before
	// this absolute set. Folding the row lock into the lookup that was already being
	// issued keeps the same ordering guarantee in one round trip instead of two: a
	// settlement that already holds the wallet row must commit before this absolute
	// value overwrites it, and a concurrent settle waits until the admin write
	// commits. SQLite has no FOR UPDATE; its single writer serialises the UPDATE.
	lookup := `SELECT id, COALESCE(status, 'active') FROM users WHERE username = ?`
	if r.db.Dialect() == "postgres" {
		lookup += ` FOR UPDATE`
	}
	if err := tx.QueryRow(lookup, username).Scan(&userID, &status); err != nil {
		return err
	}
	if status == "deleted" {
		return sql.ErrNoRows
	}
	if err := requireUserUpdated(tx.Exec(`UPDATE users SET balance = ?, balance_units = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'deleted'`, balance, newUnits, userID)); err != nil {
		return err
	}
	return tx.Commit()
}
