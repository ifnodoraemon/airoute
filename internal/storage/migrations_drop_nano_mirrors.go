package storage

import "fmt"

// legacyNanoMirror couples a deprecated *_nano money column with the *_units
// column that replaced it. Both store 1e-9 CNY.
type legacyNanoMirror struct {
	table string
	nano  string
	units string
}

// legacyNanoMirrors is the complete set of money mirrors left behind by migration
// 12. The live database carried exactly these four.
var legacyNanoMirrors = []legacyNanoMirror{
	{"users", "balance_nano", "balance_units"},
	{"api_keys", "used_cost_nano", "used_cost_units"},
	{"api_keys", "budget_nano", "budget_units"},
	{"usage_logs", "cost_nano", "cost_units"},
}

// dropLegacyNanoMoneyColumns removes the deprecated *_nano money mirrors.
//
// Migration 12 stored money as integer nanoyuan columns named *_nano. They were
// superseded by *_units, and the writers only ever update *_units, so every row
// ended up carrying two "authoritative" values. On the live cluster
// api_keys.used_cost_nano still held 23.0794 while used_cost_units held 493.41
// for the same row: any query that trusted the mirror would under-report by 13x.
// The columns are dropped rather than kept as a trap.
//
// Safety, verified before the drop: no runtime query references these columns
// (only migrations do), no index or constraint covers them, every INSERT uses an
// explicit column list, and no row had a value that existed only in the mirror.
// The backfill below still runs first so a database restored from an older dump
// cannot lose a mirror-only amount on the way out.
func dropLegacyNanoMoneyColumns(db *DB) error {
	for _, m := range legacyNanoMirrors {
		if !tableHasColumn(db, m.table, m.nano) {
			continue
		}
		// Dropping the mirror without a readable *_units column would destroy the
		// only copy of the amount. Defer and let a later start retry once the
		// columns exist instead of guessing.
		if !tableHasColumn(db, m.table, m.units) {
			return fmt.Errorf("%w: %s.%s exists without %s", errMigrationDeferred, m.table, m.nano, m.units)
		}
		if _, err := db.Exec(fmt.Sprintf(
			`UPDATE %s SET %s = %s WHERE COALESCE(%s, 0) = 0 AND COALESCE(%s, 0) <> 0`,
			m.table, m.units, m.nano, m.units, m.nano,
		)); err != nil {
			return fmt.Errorf("backfill %s.%s into %s: %w", m.table, m.nano, m.units, err)
		}
		if err := dropColumnIfExists(db, m.table, m.nano); err != nil {
			return fmt.Errorf("drop %s.%s: %w", m.table, m.nano, err)
		}
	}
	return nil
}

// dropColumnIfExists drops a column on either dialect. Table and column names
// come from legacyNanoMirrors, never from input.
func dropColumnIfExists(db *DB, table, column string) error {
	if !tableHasColumn(db, table, column) {
		return nil
	}
	stmt := fmt.Sprintf(`ALTER TABLE %s DROP COLUMN IF EXISTS %s`, table, column)
	if db.Dialect() != "postgres" {
		// SQLite has no IF EXISTS for DROP COLUMN; existence was checked above.
		stmt = fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, table, column)
	}
	_, err := db.Exec(stmt)
	return err
}
