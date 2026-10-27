package storage

import "context"

// VideoTask records the upstream selected at creation. It is shared by replicas.
type VideoTask struct {
	ID, UpstreamID, Owner, Model, ChannelName, BaseURL string
	ChannelID                                          int64
}

func createVideoTasks(db *DB) error {
	_, err := db.DB.Exec(`CREATE TABLE IF NOT EXISTS video_tasks (
		id TEXT PRIMARY KEY, upstream_id TEXT NOT NULL, owner TEXT NOT NULL,
		model TEXT NOT NULL, channel_id BIGINT NOT NULL, channel_name TEXT NOT NULL,
		base_url TEXT NOT NULL, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (r *Repository) SaveVideoTask(ctx context.Context, task *VideoTask) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO video_tasks
		(id, upstream_id, owner, model, channel_id, channel_name, base_url) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.UpstreamID, task.Owner, task.Model, task.ChannelID, task.ChannelName, task.BaseURL)
	return err
}

func (r *Repository) GetVideoTask(ctx context.Context, id, owner string) (*VideoTask, error) {
	var task VideoTask
	err := r.db.QueryRowContext(ctx, `SELECT id, upstream_id, owner, model, channel_id, channel_name, base_url
		FROM video_tasks WHERE id = ? AND owner = ?`, id, owner).Scan(
		&task.ID, &task.UpstreamID, &task.Owner, &task.Model, &task.ChannelID, &task.ChannelName, &task.BaseURL)
	return &task, err
}
