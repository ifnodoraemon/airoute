package storage

import (
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// addUsageLogFTSIndex maintains SQLite's external-content trigram index and all
// three write triggers. The path is a separate searchable field after v52.
func addUsageLogFTSIndex(db *DB) error {
	if db.Dialect() != "sqlite" {
		return nil
	}
	columns := []string{"trace_id", "chat_id", "session_id", "model", "channel", "error_message"}
	trailColumn := tableHasColumn(db, "usage_logs", "channel_trail")
	if trailColumn {
		columns = append(columns, "channel_trail")
	}
	var existing int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='usage_logs_fts'`).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 && trailColumn {
		var schema string
		if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='usage_logs_fts'`).Scan(&schema); err != nil {
			return err
		}
		if !strings.Contains(schema, "channel_trail") {
			for _, trigger := range []string{"ai", "ad", "au"} {
				if _, err := db.Exec(`DROP TRIGGER IF EXISTS usage_logs_fts_` + trigger); err != nil {
					return err
				}
			}
			if _, err := db.Exec(`DROP TABLE usage_logs_fts`); err != nil {
				return err
			}
			existing = 0
		}
	}
	names := strings.Join(columns, ", ")
	if existing == 0 {
		if _, err := db.Exec(`CREATE VIRTUAL TABLE usage_logs_fts USING fts5(` + names + `, content='usage_logs', content_rowid='id', tokenize='trigram')`); err != nil {
			telemetry.ScopedLogger("system").Warn("SQLite FTS5 trigram index unavailable; audit keyword search keeps the LIKE scan", "error", err.Error())
			return nil
		}
	}
	oldValues, newValues := []string{"old.id"}, []string{"new.id"}
	for _, column := range columns {
		oldValues = append(oldValues, "old."+column)
		newValues = append(newValues, "new."+column)
	}
	insert := `INSERT INTO usage_logs_fts(rowid, ` + names + `) VALUES (` + strings.Join(newValues, ", ") + `);`
	remove := `INSERT INTO usage_logs_fts(usage_logs_fts, rowid, ` + names + `) VALUES ('delete', ` + strings.Join(oldValues, ", ") + `);`
	statements := []string{
		`DROP TRIGGER IF EXISTS usage_logs_fts_ai`, `DROP TRIGGER IF EXISTS usage_logs_fts_ad`, `DROP TRIGGER IF EXISTS usage_logs_fts_au`,
		`CREATE TRIGGER usage_logs_fts_ai AFTER INSERT ON usage_logs BEGIN ` + insert + ` END`,
		`CREATE TRIGGER usage_logs_fts_ad AFTER DELETE ON usage_logs BEGIN ` + remove + ` END`,
		`CREATE TRIGGER usage_logs_fts_au AFTER UPDATE OF ` + names + ` ON usage_logs BEGIN ` + remove + insert + ` END`,
		`INSERT INTO usage_logs_fts(usage_logs_fts) VALUES('rebuild')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("prepare SQLite audit search index: %w", err)
		}
	}
	return nil
}
