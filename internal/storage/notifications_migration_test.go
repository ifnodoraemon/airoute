package storage

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNotificationsSchemaAndPublicationRollback(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	r := &Repository{db: db}
	ctx := context.Background()
	require.NoError(t, r.SetSetting("system_announcement", "obsolete"))
	require.NoError(t, createNotifications(db))
	require.NoError(t, createNotifications(db))
	require.Equal(t, "removed", r.GetSetting("system_announcement", "removed"))
	draft := AnnouncementRecord{Title: "公告", Content: "正文", Level: "warning", Status: "draft"}
	rec, err := r.CreateAnnouncement(ctx, "rollback", draft)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER fail_notification BEFORE INSERT ON notifications BEGIN SELECT RAISE(ABORT,'test insert failure'); END`)
	require.NoError(t, err)
	draft.Status = "published"
	_, err = r.UpdateAnnouncement(ctx, rec.ID, draft)
	require.Error(t, err)
	saved, err := r.GetAnnouncement(ctx, rec.ID)
	require.NoError(t, err)
	require.Equal(t, "draft", saved.Status)
	require.Nil(t, saved.PublishedAt)
	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notifications").Scan(&count))
	require.Zero(t, count)
}
