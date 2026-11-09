package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SendNotification uses the caller's request ID to make network retries safe.
// The durable account ID, rather than the display name, owns a private message.
func (r *Repository) SendNotification(ctx context.Context, requestID, recipientUsername, title, content, level string) (*NotificationRecord, error) {
	if recipientUsername == "" {
		return nil, ErrNotificationRecipient
	}
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rec := &NotificationRecord{ID: "msg_" + requestID, Kind: "message", Title: title, Content: content, Level: level}
	var userID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(username) = LOWER(?) AND status <> 'deleted'`, recipientUsername).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotificationRecipient
	}
	if err != nil {
		return nil, err
	}
	rec.RecipientUserID = &userID
	_, err = tx.ExecContext(ctx, `INSERT INTO notifications (id, kind, title, content, level, recipient_user_id, created_at)
		VALUES (?, 'message', ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`, rec.ID, title, content, level,
		rec.RecipientUserID, r.dialectTimePrecise(time.Now()))
	if err != nil {
		return nil, err
	}
	var saved NotificationRecord
	var recipient sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT title, content, level, recipient_user_id, created_at FROM notifications WHERE id = ?`, rec.ID).
		Scan(&saved.Title, &saved.Content, &saved.Level, &recipient, &rec.CreatedAt)
	if err != nil {
		return nil, err
	}
	if saved.Title != title || saved.Content != content || saved.Level != level ||
		recipient.Valid != (rec.RecipientUserID != nil) || (recipient.Valid && recipient.Int64 != *rec.RecipientUserID) {
		return nil, ErrNotificationConflict
	}
	return rec, tx.Commit()
}

func (r *Repository) MarkNotificationRead(ctx context.Context, userID int64, id string) error {
	if userID <= 0 {
		return fmt.Errorf("notification user is required")
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO notification_reads (user_id, notification_id, read_at)
		SELECT ?, n.id, ? FROM notifications n WHERE n.id = ? AND `+notificationVisibility+`
		ON CONFLICT(user_id, notification_id) DO UPDATE SET read_at = notification_reads.read_at`,
		userID, r.dialectTimePrecise(time.Now()), id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotificationNotFound
	}
	return err
}

// MarkAllNotificationsRead only consumes the snapshot the user received. A new
// delivery arriving while the request is in flight must remain unread.
func (r *Repository) MarkAllNotificationsRead(ctx context.Context, userID, throughSequence int64, kind string) error {
	if userID <= 0 || throughSequence <= 0 || (kind != "announcement" && kind != "message") {
		return fmt.Errorf("notification user and snapshot are required")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO notification_reads (user_id, notification_id, read_at)
		SELECT ?, n.id, ? FROM notifications n WHERE n.sequence <= ? AND n.kind = ? AND `+notificationVisibility+`
		ON CONFLICT(user_id, notification_id) DO NOTHING`, userID, r.dialectTimePrecise(time.Now()), throughSequence, kind, userID)
	return err
}
