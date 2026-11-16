package storage

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// Historical migration tests must explicitly reconstruct the old schema; the
// current runtime has no float money columns or fallback reads.
func restoreFloatMoneyTestColumns(t *testing.T, db *DB) {
	t.Helper()
	for _, m := range floatMoneyMirrors {
		if !tableHasColumn(db, m.table, m.column) {
			_, err := db.Exec("ALTER TABLE " + m.table + " ADD COLUMN " + m.column + " DOUBLE PRECISION DEFAULT 0")
			require.NoError(t, err)
		}
	}
}

func rawMoneyMigrationDB(t *testing.T, dialect, dsn string) *DB {
	t.Helper()
	driver := dialect
	if driver == "sqlite" {
		dsn = ":memory:"
	}
	raw, err := sql.Open(driver, dsn)
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	db := &DB{DB: raw, dialect: dialect}
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, statement := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, balance DOUBLE PRECISION, balance_units BIGINT DEFAULT 0)`,
		`CREATE TABLE api_keys (id INTEGER PRIMARY KEY, budget DOUBLE PRECISION, budget_units BIGINT DEFAULT 0, used_cost DOUBLE PRECISION, used_cost_units BIGINT DEFAULT 0)`,
		`CREATE TABLE usage_logs (id INTEGER PRIMARY KEY, cost DOUBLE PRECISION, cost_units BIGINT DEFAULT 0)`,
	} {
		_, err := db.Exec(statement)
		require.NoError(t, err)
	}
	return db
}

func checkDropFloatMoney(t *testing.T, db *DB) {
	t.Helper()
	for _, m := range floatMoneyMirrors {
		// Nonzero units win over a stale display mirror, including negative debt.
		_, err := db.Exec("INSERT INTO " + m.table + " (id, " + m.column + ", " + m.units + ") VALUES (1, 1.234567891, 0), (2, 99, -2000000003) ON CONFLICT (id) DO UPDATE SET " + m.column + "=excluded." + m.column + ", " + m.units + "=excluded." + m.units)
		require.NoError(t, err)
	}
	require.NoError(t, dropFloatMoneyColumns(db))
	require.NoError(t, dropFloatMoneyColumns(db), "a crash before recording v55 must be replayable")
	for _, m := range floatMoneyMirrors {
		require.False(t, tableHasColumn(db, m.table, m.column), "%s.%s survived", m.table, m.column)
		var legacy, current int64
		require.NoError(t, db.QueryRow("SELECT "+m.units+" FROM "+m.table+" WHERE id=1").Scan(&legacy))
		require.NoError(t, db.QueryRow("SELECT "+m.units+" FROM "+m.table+" WHERE id=2").Scan(&current))
		require.Equal(t, int64(1234567891), legacy, "legacy amount must survive")
		require.Equal(t, int64(-2000000003), current, "integer amount must stay authoritative")
	}
}

func TestDropFloatMoneyColumns(t *testing.T) {
	checkDropFloatMoney(t, rawMoneyMigrationDB(t, "sqlite", ""))
}

func TestDropFloatMoneyColumnsRollsBackOnOverflow(t *testing.T) {
	checkFloatMoneyRollback(t, rawMoneyMigrationDB(t, "sqlite", ""))
}

func checkFloatMoneyRollback(t *testing.T, db *DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users (id, balance) VALUES (1, 1.25)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO usage_logs (id, cost) VALUES (1, 1e20)`)
	require.NoError(t, err)
	require.ErrorContains(t, dropFloatMoneyColumns(db), "outside integer currency units")
	for _, m := range floatMoneyMirrors {
		require.True(t, tableHasColumn(db, m.table, m.column), "failed migration must restore all DDL")
	}
	var units int64
	require.NoError(t, db.QueryRow(`SELECT balance_units FROM users WHERE id=1`).Scan(&units))
	require.Zero(t, units, "failed migration must also roll back backfills")
}

func TestCurrentMoneySchemaHasNoFloatMirrors(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, m := range floatMoneyMirrors {
		require.False(t, tableHasColumn(db, m.table, m.column))
	}
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=55`).Scan(&count))
	require.Equal(t, 1, count)
}
