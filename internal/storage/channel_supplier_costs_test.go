package storage

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelRenameSupplierCosts(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	channelRenameSupplierSuite(t, db)
}

func channelRenameSupplierSuite(t *testing.T, db *DB) {
	r := NewRepository(db)
	for _, fullUpdate := range []bool{false, true} {
		t.Run(fmt.Sprintf("full_update_%t", fullUpdate), func(t *testing.T) {
			oldName := fmt.Sprintf("rename-%d", time.Now().UnixNano())
			newName := oldName + "-new"
			ch := &ChannelRecord{Name: oldName, Type: "openai", BaseURL: "http://example.invalid", Status: "active"}
			require.NoError(t, r.CreateChannel(ch))
			t.Cleanup(func() {
				_, _ = db.Exec(`DELETE FROM supplier_costs WHERE channel IN (?, ?)`, oldName, newName)
				_, _ = db.Exec(`DELETE FROM channels WHERE id = ?`, ch.ID)
			})
			renamed := func() error {
				if fullUpdate {
					copy := *ch
					copy.Name = newName
					return r.UpdateChannel(&copy)
				}
				_, err := r.PatchChannel(ch.ID, func(row *ChannelRecord) error { row.Name = newName; return nil })
				return err
			}
			require.NoError(t, r.UpsertSupplierCost(&SupplierCostRecord{Channel: oldName, Model: "m", InputCostUnitsPerMillion: 11, Status: "disabled"}))
			require.NoError(t, r.UpsertSupplierCost(&SupplierCostRecord{Channel: newName, Model: "m", InputCostUnitsPerMillion: 22}))
			require.ErrorIs(t, renamed(), ErrSupplierCostRenameConflict)
			stored, err := findChannel(t, r, ch.ID)
			require.NoError(t, err)
			require.Equal(t, oldName, stored.Name)
			quote, err := r.GetSupplierCost(oldName, "m")
			require.NoError(t, err)
			require.EqualValues(t, 11, quote.InputCostUnitsPerMillion)
			other, err := r.GetSupplierCost(newName, "m")
			require.NoError(t, err)
			require.EqualValues(t, 22, other.InputCostUnitsPerMillion)
			require.NoError(t, r.DeleteSupplierCost(other.ID))

			// A storage failure must roll back both sides of the rename.
			guard := fmt.Sprintf("rename_guard_%d", ch.ID)
			if db.Dialect() == "sqlite" {
				_, err = db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON supplier_costs BEGIN SELECT RAISE(ABORT, 'injected quote failure'); END`, guard))
			} else {
				_, err = db.Exec(fmt.Sprintf(`ALTER TABLE supplier_costs ADD CONSTRAINT %s CHECK (channel <> '%s')`, guard, newName))
			}
			require.NoError(t, err)
			require.Error(t, renamed())
			stored, err = findChannel(t, r, ch.ID)
			require.NoError(t, err)
			require.Equal(t, oldName, stored.Name)
			if db.Dialect() == "sqlite" {
				_, err = db.Exec(`DROP TRIGGER ` + guard)
			} else {
				_, err = db.Exec(`ALTER TABLE supplier_costs DROP CONSTRAINT ` + guard)
			}
			require.NoError(t, err)

			require.NoError(t, renamed())
			_, err = r.GetSupplierCost(oldName, "m")
			require.ErrorIs(t, err, sql.ErrNoRows)
			moved, err := r.GetSupplierCost(newName, "m")
			require.NoError(t, err)
			require.Equal(t, quote.ID, moved.ID)
			require.EqualValues(t, 11, moved.InputCostUnitsPerMillion)
			require.Equal(t, "disabled", moved.Status)
		})
	}
}
