package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrNotificationNotFound  = errors.New("notification not found")
	ErrNotificationRecipient = errors.New("notification recipient not found")
	ErrNotificationConflict  = errors.New("notification request already used with different content")
)

const notificationVisibility = `(n.kind = 'announcement' OR n.recipient_user_id = ?)`

func (r *Repository) NotificationUnreadCounts(ctx context.Context, userID int64) (*NotificationCounts, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("notification user is required")
	}
	counts := &NotificationCounts{}
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN n.kind = 'announcement' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN n.kind = 'message' THEN 1 ELSE 0 END), 0) FROM notifications n
		LEFT JOIN notification_reads nr ON nr.notification_id = n.id AND nr.user_id = ?
		WHERE `+notificationVisibility+` AND nr.notification_id IS NULL`, userID, userID).
		Scan(&counts.UnreadCount, &counts.AnnouncementUnreadCount, &counts.MessageUnreadCount)
	return counts, err
}

func (r *Repository) ListNotifications(ctx context.Context, userID int64, filter NotificationFilter) (*NotificationPage, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("notification user is required")
	}
	if filter.Page < 1 || filter.Page > 100000 || filter.PageSize < 1 || filter.PageSize > 100 {
		return nil, fmt.Errorf("invalid notification pagination")
	}
	page := &NotificationPage{Items: []NotificationRecord{}, Page: filter.Page, PageSize: filter.PageSize}
	join := ` FROM notifications n LEFT JOIN notification_reads nr ON nr.notification_id = n.id AND nr.user_id = ? WHERE ` + notificationVisibility
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(n.sequence), 0)`+join,
		userID, userID).Scan(&page.LatestSequence)
	if err != nil {
		return nil, err
	}
	counts, err := r.NotificationUnreadCounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	page.NotificationCounts = *counts
	where, args := notificationQuery(filter, page.LatestSequence, userID)
	if err = r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+join+where, args...).Scan(&page.Total); err != nil {
		return nil, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT n.id, n.kind, COALESCE(n.announcement_id, ''), n.title, n.content, n.level, n.created_at, nr.read_at`+
		join+where+` ORDER BY n.sequence DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rec NotificationRecord
		var readAt sql.NullTime
		if err = rows.Scan(&rec.ID, &rec.Kind, &rec.AnnouncementID, &rec.Title, &rec.Content, &rec.Level, &rec.CreatedAt, &readAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			rec.ReadAt = &readAt.Time
		}
		page.Items = append(page.Items, rec)
	}
	return page, rows.Err()
}

func notificationQuery(filter NotificationFilter, latest, userID int64) (string, []any) {
	where := " AND n.sequence <= ?"
	args := []any{userID, userID, latest}
	if filter.Kind != "" {
		where += " AND n.kind = ?"
		args = append(args, filter.Kind)
	}
	if filter.Unread {
		where += " AND nr.notification_id IS NULL"
	}
	return where, args
}
