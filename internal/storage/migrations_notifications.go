package storage

import "fmt"

func createNotifications(db *DB) error {
	id, timestamp := "INTEGER PRIMARY KEY AUTOINCREMENT", "DATETIME"
	if db.Dialect() == "postgres" {
		id, timestamp = "BIGSERIAL PRIMARY KEY", "TIMESTAMPTZ"
	}
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS announcements (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, content TEXT NOT NULL,
			level TEXT NOT NULL CHECK (level IN ('info','warning','critical')),
			status TEXT NOT NULL CHECK (status IN ('draft','published')),
			show_banner BOOLEAN NOT NULL DEFAULT FALSE,
			created_at %[2]s NOT NULL, updated_at %[2]s NOT NULL, published_at %[2]s
		);
		CREATE INDEX IF NOT EXISTS idx_announcements_status ON announcements(status, updated_at);
		CREATE TABLE IF NOT EXISTS notifications (
			sequence %[1]s, id TEXT UNIQUE NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('announcement','message')),
			announcement_id TEXT REFERENCES announcements(id) ON DELETE CASCADE,
			title TEXT NOT NULL, content TEXT NOT NULL,
			level TEXT NOT NULL CHECK (level IN ('info','warning','critical')),
			recipient_user_id BIGINT REFERENCES users(id), created_at %[2]s NOT NULL,
			CHECK ((kind='announcement' AND recipient_user_id IS NULL AND announcement_id IS NOT NULL)
				OR (kind='message' AND recipient_user_id IS NOT NULL AND announcement_id IS NULL))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_announcement ON notifications(announcement_id);
		CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications(recipient_user_id, sequence);
		CREATE INDEX IF NOT EXISTS idx_notifications_kind ON notifications(kind, sequence);
		CREATE TABLE IF NOT EXISTS notification_reads (
			user_id BIGINT NOT NULL REFERENCES users(id),
			notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
			read_at %[2]s NOT NULL, PRIMARY KEY (user_id, notification_id)
		);
		DELETE FROM system_settings WHERE key='system_announcement';
	`, id, timestamp))
	return err
}
