package storage

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// defaultMaxOverdraftCNY bounds how far a wallet may go negative on a single
// charge (and, because the floor is enforced in SQL, across concurrent charges).
//
// Prepaid wallets had no floor: `balance = balance - cost` let an expensive
// request, or a burst admitted inside the 10-second snapshot window, drive a
// wallet arbitrarily negative with no alert. One yuan is a deliberate, tiny
// forgiveness budget per account — large enough that a rounding-level overdraft
// still settles, small enough that an account cannot run up a real debt.
const defaultMaxOverdraftCNY = 1.0

// maxOverdraftUnits returns the permitted overdraft as a positive number of 1e-9
// units. Callers negate it to form the floor for a wallet deduction. An
// unparsable or negative setting means "no overdraft allowed".
func maxOverdraftUnits() int64 {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_MAX_OVERDRAFT_CNY"))
	if raw == "" {
		return ToUnits(defaultMaxOverdraftCNY)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return 0
	}
	return ToUnits(value)
}

// walletUnitsExpr computes the wallet's new integer balance, clamped at the
// overdraft floor. It is written as a portable CASE rather than GREATEST/MAX so
// the same statement works on SQLite and PostgreSQL.
const walletUnitsExpr = `CASE WHEN COALESCE(balance_units, 0) - ? < ? THEN ? ELSE COALESCE(balance_units, 0) - ? END`

// deductWallet applies one clamped wallet deduction and returns whether a row was
// updated plus the resulting balance in 1e-9 units.
//
// applied=false means no user matched the WHERE clause (an admin-exempt account
// or an unknown key), which is what the callers' tenant-fallback logic keys on.
//
// The float `balance` column is derived from the integer column in the same
// statement so the two mirrors can never drift, and the floor is applied there
// too. An overdraft that reaches the floor is counted and logged (rate limited)
// instead of passing silently — the operator needs to see accounts that are
// being served on credit.
func (r *Repository) deductWallet(tx *Tx, where string, whereArgs []any, cost float64) (bool, int64, error) {
	if tx == nil || cost <= 0 {
		return false, 0, nil
	}
	units := ToUnits(cost)
	if units <= 0 {
		return false, 0, nil
	}
	floor := -maxOverdraftUnits()

	query := `UPDATE users SET balance_units = ` + walletUnitsExpr +
		`, balance = (` + walletUnitsExpr + `) * 1.0 / 1000000000.0, updated_at = CURRENT_TIMESTAMP WHERE ` +
		where + ` RETURNING balance_units`

	// Eight placeholders: four for the integer assignment, four for the float one.
	args := make([]any, 0, 8+len(whereArgs))
	for i := 0; i < 2; i++ {
		args = append(args, units, floor, floor, units)
	}
	args = append(args, whereArgs...)

	ledgerReady := walletLedgerReadyTx(tx)
	if err := r.lockWalletUserRows(tx, where, whereArgs); err != nil {
		return false, 0, err
	}

	var oldUnits int64
	oldKnown := false
	if ledgerReady {
		if err := tx.QueryRow(`SELECT COALESCE(balance_units, 0) FROM users WHERE `+where, whereArgs...).Scan(&oldUnits); err == nil {
			oldKnown = true
		}
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()

	applied := false
	var balanceUnits int64
	if rows.Next() {
		if err := rows.Scan(&balanceUnits); err != nil {
			return false, 0, err
		}
		applied = true
	}
	if err := rows.Err(); err != nil {
		return false, 0, err
	}

	if applied && ledgerReady && oldKnown && balanceUnits != oldUnits {
		var userID int64
		if err := tx.QueryRow(`SELECT id FROM users WHERE `+where, whereArgs...).Scan(&userID); err != nil {
			return false, 0, err
		}
		if err := r.appendWalletLedger(tx, userID, WalletTypeCharge, balanceUnits-oldUnits, balanceUnits, "", "usage charge", ""); err != nil {
			return false, 0, err
		}
	}

	if applied && balanceUnits < 0 {
		telemetry.GlobalMetrics.BillingOverdrafts.Add(1)
		logger := telemetry.ScopedLogger("billing")
		if balanceUnits <= floor {
			telemetry.WarnLogger(logger, "wallet-overdraft-clamped", time.Minute,
				"wallet charge clamped at the overdraft floor",
				"cost_cny", cost,
				"balance_cny", FromUnits(balanceUnits),
				"floor_cny", FromUnits(floor),
			)
		} else {
			telemetry.WarnLogger(logger, "wallet-overdraft", time.Minute,
				"wallet balance went negative after a charge",
				"cost_cny", cost,
				"balance_cny", FromUnits(balanceUnits),
			)
		}
	}
	return applied, balanceUnits, nil
}

// lockWalletUserRows serializes balance changes for the matched users before the
// caller reads the old balance. PostgreSQL uses FOR UPDATE; SQLite has no row
// locks, so a no-op update acquires the database write lock instead. This keeps
// the ledger delta equal to the balance movement even when a settlement and an
// admin adjustment race on the same account.
func (r *Repository) lockWalletUserRows(tx *Tx, where string, whereArgs []any) error {
	if tx == nil {
		return nil
	}
	if r.db.Dialect() == "postgres" {
		rows, err := tx.Query(`SELECT id FROM users WHERE `+where+` FOR UPDATE`, whereArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	_, err := tx.Exec(`UPDATE users SET updated_at = updated_at WHERE `+where, whereArgs...)
	return err
}
