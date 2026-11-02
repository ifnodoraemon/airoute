package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// retireTextCompletions removes retired capability declarations. A text-only
// channel is disabled; leaving it enabled with [] would mean "all protocols".
func retireTextCompletions(db *DB) error {
	if exists, err := tableExists(db, "channels"); err != nil || !exists {
		return err
	}
	rows, err := db.Query(`SELECT id, protocols, status FROM channels`)
	if err != nil {
		return err
	}
	type change struct {
		id        int64
		protocols string
		status    string
	}
	var changes []change
	for rows.Next() {
		var id int64
		var raw sql.NullString
		var status string
		if err := rows.Scan(&id, &raw, &status); err != nil {
			rows.Close()
			return err
		}
		var protocols []string
		if !raw.Valid || json.Unmarshal([]byte(raw.String), &protocols) != nil {
			continue
		}
		kept := make([]string, 0, len(protocols))
		for _, protocol := range protocols {
			if !model.IsRetiredProtocol(protocol) {
				kept = append(kept, protocol)
			}
		}
		if len(kept) == len(protocols) {
			continue
		}
		if len(kept) == 0 {
			status = "disabled"
		}
		body, _ := json.Marshal(kept)
		changes = append(changes, change{id: id, protocols: string(body), status: status})
	}
	readErr := rows.Err()
	rows.Close()
	if readErr != nil {
		return readErr
	}
	for _, change := range changes {
		if _, err := db.Exec(`UPDATE channels SET protocols=?, status=? WHERE id=?`, change.protocols, change.status, change.id); err != nil {
			return fmt.Errorf("retire completions for channel %d: %w", change.id, err)
		}
	}
	return nil
}
