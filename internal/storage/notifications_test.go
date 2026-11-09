package storage

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotifications(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	notificationStorageSuite(t, db)
}

func notificationStorageSuite(t *testing.T, db *DB) {
	r := &Repository{db: db}
	ctx := context.Background()
	a, b := notificationTestUser(t, r), notificationTestUser(t, r)
	send := func(key, username, title string) *NotificationRecord {
		t.Helper()
		rec, err := r.SendNotification(ctx, key, username, title, "第一段\n\n第二段", "warning")
		require.NoError(t, err)
		return rec
	}
	list := func(userID int64, kind string, unread bool) *NotificationPage {
		t.Helper()
		page, err := r.ListNotifications(ctx, userID, NotificationFilter{Kind: kind, Unread: unread, Page: 1, PageSize: 100})
		require.NoError(t, err)
		return page
	}

	t.Run("private ownership and idempotent delivery", func(t *testing.T) {
		key := rand.Text()
		rec := send(key, a.Username, "个人消息")
		require.Equal(t, rec.ID, send(key, a.Username, "个人消息").ID)
		require.EqualValues(t, 1, list(a.ID, "message", false).Total)
		require.Zero(t, list(b.ID, "message", false).Total)
		counts, err := r.NotificationUnreadCounts(ctx, b.ID)
		require.NoError(t, err)
		require.Zero(t, counts.UnreadCount)
		require.ErrorIs(t, r.MarkNotificationRead(ctx, b.ID, rec.ID), ErrNotificationNotFound)
		_, err = r.SendNotification(ctx, key, b.Username, "个人消息", "第一段\n\n第二段", "warning")
		require.ErrorIs(t, err, ErrNotificationConflict)
		_, err = r.SendNotification(ctx, rand.Text(), "missing_"+rand.Text(), "标题", "内容", "info")
		require.ErrorIs(t, err, ErrNotificationRecipient)
		require.Error(t, r.MarkNotificationRead(ctx, 0, rec.ID))
		require.NoError(t, r.MarkNotificationRead(ctx, a.ID, rec.ID))
		readAt := list(a.ID, "message", false).Items[0].ReadAt
		require.NotNil(t, readAt)
		require.NoError(t, r.MarkNotificationRead(ctx, a.ID, rec.ID))
		require.Equal(t, readAt, list(a.ID, "message", false).Items[0].ReadAt)
	})

	t.Run("independent posts drafts edits and withdrawals", func(t *testing.T) {
		draft := AnnouncementRecord{Title: "草稿", Content: "暂不发布", Level: "info", Status: "draft"}
		first, err := r.CreateAnnouncement(ctx, rand.Text(), draft)
		require.NoError(t, err)
		require.Zero(t, list(a.ID, "announcement", false).Total)
		draft.Status = "published"
		first, err = r.UpdateAnnouncement(ctx, first.ID, draft)
		require.NoError(t, err)
		firstDelivery := list(a.ID, "announcement", false).Items[0]
		require.NoError(t, r.MarkNotificationRead(ctx, a.ID, firstDelivery.ID))
		draft.ShowBanner = true
		_, err = r.UpdateAnnouncement(ctx, first.ID, draft)
		require.NoError(t, err)
		require.Zero(t, list(a.ID, "announcement", true).Total)
		second, err := r.CreateAnnouncement(ctx, rand.Text(), AnnouncementRecord{Title: "另一条公告", Content: "独立内容", Level: "info", Status: "published"})
		require.NoError(t, err)
		require.NotEqual(t, first.ID, second.ID)
		draft.Title = "编辑后的公告"
		_, err = r.UpdateAnnouncement(ctx, first.ID, draft)
		require.NoError(t, err)
		page := list(b.ID, "announcement", false)
		require.EqualValues(t, 2, page.Total)
		require.Equal(t, "编辑后的公告", page.Items[0].Title)
		require.NotEqual(t, firstDelivery.ID, page.Items[0].ID)
		require.Equal(t, first.ID, page.Items[0].AnnouncementID)
		draft.Status = "draft"
		_, err = r.UpdateAnnouncement(ctx, first.ID, draft)
		require.NoError(t, err)
		require.EqualValues(t, 1, list(a.ID, "announcement", false).Total)
		banner, err := r.GetAnnouncementBanner(ctx)
		require.NoError(t, err)
		require.Nil(t, banner)
		require.NoError(t, r.DeleteAnnouncement(ctx, first.ID))
		_, err = r.GetAnnouncement(ctx, first.ID)
		require.ErrorIs(t, err, ErrAnnouncementNotFound)
	})

	t.Run("read all respects category snapshot and account", func(t *testing.T) {
		old := send(rand.Text(), a.Username, "旧消息")
		snapshot := list(a.ID, "message", false).LatestSequence
		fresh := send(rand.Text(), a.Username, "并发新消息")
		send(rand.Text(), b.Username, "另一账号消息")
		require.NoError(t, r.MarkAllNotificationsRead(ctx, a.ID, snapshot, "message"))
		unread := list(a.ID, "message", true)
		require.EqualValues(t, 1, unread.Total)
		require.Equal(t, fresh.ID, unread.Items[0].ID)
		require.NotEqual(t, old.ID, unread.Items[0].ID)
		require.EqualValues(t, 1, list(b.ID, "message", true).Total)
		require.EqualValues(t, 1, list(a.ID, "announcement", true).Total)
		page, err := r.ListNotifications(ctx, a.ID, NotificationFilter{Kind: "message", Page: 2, PageSize: 1})
		require.NoError(t, err)
		require.EqualValues(t, 3, page.Total)
		require.Len(t, page.Items, 1)
		require.Equal(t, old.ID, page.Items[0].ID)
	})

	t.Run("concurrent replicas deliver once", func(t *testing.T) {
		draft := AnnouncementRecord{Title: "并发发布", Content: rand.Text(), Level: "critical", Status: "published"}
		key := rand.Text()
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for range 4 {
			wg.Go(func() {
				replica := &Repository{db: db}
				_, err := replica.CreateAnnouncement(ctx, key, draft)
				errs <- err
				_, err = replica.SendNotification(ctx, key, a.Username, "仅送达一次", "内容", "info")
				errs <- err
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE content = ? OR id = ?`, draft.Content, "msg_"+key).Scan(&count))
		require.Equal(t, 2, count)
	})

	t.Run("draft published after a read snapshot remains unread", func(t *testing.T) {
		draft := AnnouncementRecord{Title: "稍后发布", Content: "正文", Level: "info", Status: "draft"}
		rec, err := r.CreateAnnouncement(ctx, rand.Text(), draft)
		require.NoError(t, err)
		snapshot := list(a.ID, "announcement", false).LatestSequence
		draft.Status = "published"
		_, err = r.UpdateAnnouncement(ctx, rec.ID, draft)
		require.NoError(t, err)
		require.NoError(t, r.MarkAllNotificationsRead(ctx, a.ID, snapshot, "announcement"))
		unread := list(a.ID, "announcement", true)
		require.EqualValues(t, 1, unread.Total)
		require.Equal(t, rec.ID, unread.Items[0].AnnouncementID)
		require.NoError(t, r.MarkNotificationRead(ctx, a.ID, unread.Items[0].ID))
		deliveryID := unread.Items[0].ID
		require.NoError(t, r.DeleteAnnouncement(ctx, rec.ID))
		var reads int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notification_reads WHERE notification_id=?", deliveryID).Scan(&reads))
		require.Zero(t, reads)
	})
}

func notificationTestUser(t *testing.T, r *Repository) *UserRecord {
	t.Helper()
	user := &UserRecord{Username: "notice_" + rand.Text(), Role: "user", Status: "active", PasswordHash: "test"}
	require.NoError(t, r.CreateUser(user))
	return user
}
