//go:build integration

package storage

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgresDropFloatMoneyColumns(t *testing.T) {
	checkDropFloatMoney(t, rawMoneyMigrationDB(t, "postgres", partitionScratchDSN(t)))
}

func TestPostgresDropFloatMoneyRollback(t *testing.T) {
	checkFloatMoneyRollback(t, rawMoneyMigrationDB(t, "postgres", partitionScratchDSN(t)))
}

func TestPostgresOverviewSpendCalendarAndTenantScope(t *testing.T) {
	db, err := OpenDB(partitionScratchDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	checkOverviewSpendWindows(t, db)
}

func TestPostgresFloatMoneyRollbackScript(t *testing.T) {
	db := rawMoneyMigrationDB(t, "postgres", partitionScratchDSN(t))
	_, err := db.Exec(`INSERT INTO users (id, balance_units) VALUES (1, -1234567891);
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO schema_migrations VALUES (55, 'drop_float_money_mirrors')`)
	require.NoError(t, err)
	require.NoError(t, dropFloatMoneyColumns(db))
	script, err := os.ReadFile("../../scripts/rollback-money-mirrors-postgres.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(script))
	require.NoError(t, err)
	var display float64
	var units int64
	require.NoError(t, db.QueryRow(`SELECT balance, balance_units FROM users WHERE id=1`).Scan(&display, &units))
	require.Equal(t, -1.234567891, display)
	require.Equal(t, int64(-1234567891), units)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=55`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, dropFloatMoneyColumns(db), "rollback must permit a later upgrade")
}
