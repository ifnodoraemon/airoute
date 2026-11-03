package storage

import "errors"

// ErrSupplierCostRenameConflict prevents a rename from overwriting another
// channel/model quote. The surrounding transaction also rolls back the channel.
var ErrSupplierCostRenameConflict = errors.New("supplier quotes already exist under the new channel name")

func rebindChannelSupplierCosts(tx *Tx, rec *ChannelRecord) error {
	current, err := getChannelTx(tx, rec.ID)
	if err != nil {
		return err
	}
	if current.Name == rec.Name {
		return nil
	}
	var conflicts int
	err = tx.QueryRow(`SELECT COUNT(*) FROM supplier_costs old_quote
		JOIN supplier_costs new_quote ON new_quote.model = old_quote.model AND new_quote.channel = ?
		WHERE old_quote.channel = ?`, rec.Name, current.Name).Scan(&conflicts)
	if err != nil {
		return err
	}
	if conflicts > 0 {
		return ErrSupplierCostRenameConflict
	}
	_, err = tx.Exec(`UPDATE supplier_costs SET channel = ?, updated_at = CURRENT_TIMESTAMP WHERE channel = ?`, rec.Name, current.Name)
	return err
}
