package storage

import (
	"context"
	"crypto/rand"
	"time"
)

func announcementChanged(a, b AnnouncementRecord) bool {
	return a.Title != b.Title || a.Content != b.Content || a.Level != b.Level || a.Status != b.Status
}

func (r *Repository) CreateAnnouncement(ctx context.Context, requestID string, rec AnnouncementRecord) (*AnnouncementRecord, error) {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rec.ID, rec.CreatedAt = "ann_"+requestID, time.Now().UTC()
	rec.UpdatedAt = rec.CreatedAt
	if rec.Status == "published" {
		rec.PublishedAt = &rec.CreatedAt
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO announcements (id,title,content,level,status,show_banner,created_at,updated_at,published_at)
		VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, rec.ID, rec.Title, rec.Content, rec.Level, rec.Status,
		rec.ShowBanner, r.dialectTimePrecise(rec.CreatedAt), r.dialectTimePrecise(rec.UpdatedAt), r.announcementTime(rec.PublishedAt))
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		old, err := r.lockAnnouncement(ctx, tx, rec.ID)
		if err != nil {
			return nil, err
		}
		if announcementChanged(*old, rec) || old.ShowBanner != rec.ShowBanner {
			return nil, ErrAnnouncementConflict
		}
		return old, tx.Commit()
	}
	if rec.Status == "published" {
		if err := r.publishAnnouncement(ctx, tx, rec); err != nil {
			return nil, err
		}
	}
	return &rec, tx.Commit()
}

func (r *Repository) UpdateAnnouncement(ctx context.Context, id string, rec AnnouncementRecord) (*AnnouncementRecord, error) {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	old, err := r.lockAnnouncement(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	changed := announcementChanged(*old, rec)
	if !changed && old.ShowBanner == rec.ShowBanner {
		return old, tx.Commit()
	}
	rec.ID, rec.CreatedAt, rec.UpdatedAt, rec.PublishedAt = id, old.CreatedAt, time.Now().UTC(), old.PublishedAt
	if changed {
		if err := deleteAnnouncementDelivery(ctx, tx, id); err != nil {
			return nil, err
		}
		rec.PublishedAt = nil
		if rec.Status == "published" {
			rec.PublishedAt = &rec.UpdatedAt
			if err := r.publishAnnouncement(ctx, tx, rec); err != nil {
				return nil, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE announcements SET title=?,content=?,level=?,status=?,show_banner=?,updated_at=?,published_at=? WHERE id=?`,
		rec.Title, rec.Content, rec.Level, rec.Status, rec.ShowBanner, r.dialectTimePrecise(rec.UpdatedAt), r.announcementTime(rec.PublishedAt), id)
	if err != nil {
		return nil, err
	}
	return &rec, tx.Commit()
}

func (r *Repository) DeleteAnnouncement(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = r.lockAnnouncement(ctx, tx, id); err != nil {
		return err
	}
	if err = deleteAnnouncementDelivery(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM announcements WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) lockAnnouncement(ctx context.Context, tx *Tx, id string) (*AnnouncementRecord, error) {
	query := "SELECT " + announcementColumns + " FROM announcements WHERE id=?"
	if r.Dialect() == "postgres" {
		query += " FOR UPDATE"
	}
	return scanAnnouncement(tx.QueryRowContext(ctx, query, id))
}

func (r *Repository) announcementTime(at *time.Time) any {
	if at == nil {
		return nil
	}
	return r.dialectTimePrecise(*at)
}

func (r *Repository) publishAnnouncement(ctx context.Context, tx *Tx, rec AnnouncementRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO notifications (id,kind,announcement_id,title,content,level,created_at)
		VALUES (?,'announcement',?,?,?,?,?)`, "notice_"+rand.Text(), rec.ID, rec.Title, rec.Content, rec.Level, r.announcementTime(rec.PublishedAt))
	return err
}

func deleteAnnouncementDelivery(ctx context.Context, tx *Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_reads WHERE notification_id IN
		(SELECT id FROM notifications WHERE announcement_id=?)`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM notifications WHERE announcement_id=?", id)
	return err
}
