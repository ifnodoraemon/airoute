package storage

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrAnnouncementNotFound = errors.New("announcement not found")
	ErrAnnouncementConflict = errors.New("announcement request already used")
)

const announcementColumns = "id, title, content, level, status, show_banner, created_at, updated_at, published_at"

func scanAnnouncement(row interface{ Scan(...any) error }) (*AnnouncementRecord, error) {
	rec := &AnnouncementRecord{}
	var published sql.NullTime
	err := row.Scan(&rec.ID, &rec.Title, &rec.Content, &rec.Level, &rec.Status, &rec.ShowBanner, &rec.CreatedAt, &rec.UpdatedAt, &published)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAnnouncementNotFound
	}
	if published.Valid {
		rec.PublishedAt = &published.Time
	}
	return rec, err
}

func (r *Repository) GetAnnouncement(ctx context.Context, id string) (*AnnouncementRecord, error) {
	return scanAnnouncement(r.db.QueryRowContext(ctx, "SELECT "+announcementColumns+" FROM announcements WHERE id=?", id))
}

func (r *Repository) ListAnnouncements(ctx context.Context, status string, page, size int) (*AnnouncementPage, error) {
	out := &AnnouncementPage{Items: []AnnouncementRecord{}}
	where, args := "", []any{}
	if status != "" {
		where, args = " WHERE status=?", []any{status}
	}
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM announcements"+where, args...).Scan(&out.Total); err != nil {
		return nil, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := r.db.QueryContext(ctx, "SELECT "+announcementColumns+" FROM announcements"+where+" ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		rec, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, *rec)
	}
	return out, rows.Err()
}

func (r *Repository) GetAnnouncementBanner(ctx context.Context) (*NotificationRecord, error) {
	rec := &NotificationRecord{Kind: "announcement"}
	err := r.db.QueryRowContext(ctx, `SELECT n.id, n.announcement_id, n.title, n.content, n.level, n.created_at
		FROM notifications n JOIN announcements a ON a.id=n.announcement_id
		WHERE a.status='published' AND a.show_banner=TRUE ORDER BY n.sequence DESC LIMIT 1`).
		Scan(&rec.ID, &rec.AnnouncementID, &rec.Title, &rec.Content, &rec.Level, &rec.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}
