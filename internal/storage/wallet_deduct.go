package storage

import (
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"time"
)

// Admission prevents spending unreserved funds. Settlement always records the
// provider's actual cost; truncating debt would silently forgive delivered work.
func (r *Repository) deductWallet(tx *Tx, where string, whereArgs []any, cost float64, effects *settlementEffects) (bool, int64, error) {
	if tx == nil || cost <= 0 {
		return false, 0, nil
	}
	units := ToUnits(cost)
	if units <= 0 {
		return false, 0, nil
	}
	query := `UPDATE users SET balance_units = COALESCE(balance_units, 0) - ?, balance = (COALESCE(balance_units, 0) - ?) * 1.0 / 1000000000.0, updated_at = CURRENT_TIMESTAMP WHERE ` + where + ` RETURNING balance_units`
	args := append([]any{units, units}, whereArgs...)
	rows, err := tx.Query(query, args...)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()
	applied := false
	var balance int64
	if rows.Next() {
		if err := rows.Scan(&balance); err != nil {
			return false, 0, err
		}
		applied = true
	}
	if err := rows.Err(); err != nil {
		return false, 0, err
	}
	if applied {
		recordWalletDeductionEffect(balance, effects)
	}
	return applied, balance, nil
}

type settlementEffects struct{ overdrafts int64 }

func (e *settlementEffects) flush() {
	if e != nil && e.overdrafts > 0 {
		telemetry.GlobalMetrics.BillingOverdrafts.Add(uint64(e.overdrafts))
	}
}

func recordWalletDeductionEffect(balance int64, effects *settlementEffects) {
	if balance >= 0 {
		return
	}
	if effects != nil {
		effects.overdrafts++
	}
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "wallet-actual-cost-overdraft", time.Minute,
		"wallet actual cost exceeded available funds", "balance_units", balance)
}
