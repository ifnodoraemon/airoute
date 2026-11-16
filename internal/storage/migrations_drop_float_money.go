package storage

import "fmt"

// These four display mirrors were replaced by integer 1e-9 currency units.
// Rates and payment order amounts are separate contracts, not ledger mirrors.
var floatMoneyMirrors = []struct{ table, column, units string }{
	{"users", "balance", "balance_units"},
	{"api_keys", "budget", "budget_units"},
	{"api_keys", "used_cost", "used_cost_units"},
	{"usage_logs", "cost", "cost_units"},
}

// dropFloatMoneyColumns backfills legacy float-only rows before removing their
// mirrors. Both the data changes and DDL commit together; a failed conversion or
// DROP leaves every table intact. Replaying after a crash is safe.
func dropFloatMoneyColumns(db *DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range floatMoneyMirrors {
		exists, err := moneyColumnExists(tx, m.table, m.column)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if exists, err = moneyColumnExists(tx, m.table, m.units); err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("cannot drop %s.%s without %s", m.table, m.column, m.units)
		}
		if db.Dialect() == "postgres" {
			if _, err := tx.Exec("LOCK TABLE " + m.table + " IN ACCESS EXCLUSIVE MODE"); err != nil {
				return err
			}
		}
		if err := backfillFloatMoney(tx, m.table, m.column, m.units); err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", m.table, m.column)); err != nil {
			return fmt.Errorf("drop %s.%s: %w", m.table, m.column, err)
		}
	}
	return tx.Commit()
}

func moneyColumnExists(tx *Tx, table, column string) (bool, error) {
	query := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`
	if tx.db.Dialect() == "postgres" {
		query = `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`
	}
	var count int
	err := tx.QueryRow(query, table, column).Scan(&count)
	return count > 0, err
}

func backfillFloatMoney(tx *Tx, table, column, units string) error {
	// SQLite would silently clamp an overflowing REAL->INTEGER cast. Reject it
	// on both dialects so a migration can never erase an unrepresentable amount.
	where := fmt.Sprintf("COALESCE(%s, 0) = 0 AND COALESCE(%s, 0) <> 0", units, column)
	invalid := fmt.Sprintf("NOT (ROUND(%s * 1000000000) >= -9223372036854775808 AND ROUND(%s * 1000000000) < 9223372036854775808)", column, column)
	if tx.db.Dialect() != "postgres" {
		invalid += fmt.Sprintf(" OR typeof(%s) NOT IN ('integer', 'real')", column)
	}
	var count int
	if err := tx.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s AND (%s)", table, where, invalid)).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot migrate %s.%s: %d amounts are outside integer currency units", table, column, count)
	}
	_, err := tx.Exec(fmt.Sprintf("UPDATE %s SET %s = CAST(ROUND(%s * 1000000000) AS BIGINT) WHERE %s", table, units, column, where))
	return err
}
