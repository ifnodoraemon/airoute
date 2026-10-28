package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// VideoTask records the upstream selected at creation. It is shared by replicas.
type VideoTask struct {
	ID, UpstreamID, Owner, Model, ChannelName, BaseURL string
	ChannelID                                          int64
}

// createVideoTasks creates the channel-pin table used to poll a video task.
//
// The DDL is dialect-specific for the timestamp column: PostgreSQL's
// `TIMESTAMP` is a zone-less wall-clock value whose CURRENT_TIMESTAMP default is
// the session's local time, while SQLite's CURRENT_TIMESTAMP is UTC. Sharing one
// DDL therefore gave the two dialects different meanings for the same column, and
// the retention purge (which binds a UTC cutoff) deleted the wrong window on any
// PostgreSQL server that is not set to UTC. Every other table in this schema uses
// TIMESTAMPTZ on PostgreSQL for the same reason.
func createVideoTasks(db *DB) error {
	_, err := db.DB.Exec(videoTasksDDL(db.Dialect()))
	return err
}

// videoTasksDDL renders the table definition for a dialect. It is separate from
// createVideoTasks so the two branches can be asserted without a PostgreSQL
// server.
func videoTasksDDL(dialect string) string {
	if dialect == "postgres" {
		return `CREATE TABLE IF NOT EXISTS video_tasks (
			id TEXT PRIMARY KEY, upstream_id TEXT NOT NULL, owner TEXT NOT NULL,
			model TEXT NOT NULL, channel_id BIGINT NOT NULL, channel_name TEXT NOT NULL,
			base_url TEXT NOT NULL, created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`
	}
	return `CREATE TABLE IF NOT EXISTS video_tasks (
		id TEXT PRIMARY KEY, upstream_id TEXT NOT NULL, owner TEXT NOT NULL,
		model TEXT NOT NULL, channel_id BIGINT NOT NULL, channel_name TEXT NOT NULL,
		base_url TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`
}

// videoTaskURLCredentialParams are the query parameters that carry credentials in
// self-hosted / gateway media endpoints. They are removed before a base URL is
// persisted: the dispatcher authenticates with the channel's api_key header, so a
// credential embedded in the URL is unnecessary and would otherwise be written to
// the database in plaintext.
var videoTaskURLCredentialParams = map[string]struct{}{
	"api_key":      {},
	"apikey":       {},
	"api-key":      {},
	"key":          {},
	"token":        {},
	"access_token": {},
	"auth":         {},
	"password":     {},
	"secret":       {},
	"signature":    {},
	"sig":          {},
	"x-api-key":    {},
}

// sanitizeVideoTaskBaseURL strips credentials from an upstream base URL before it
// is persisted in video_tasks.
//
// Polling needs the address, so the URL itself cannot be dropped, but there is no
// reason to copy userinfo (user:password@) or credential query parameters into a
// long-lived row: the dispatcher authenticates with the channel's api_key header.
// The second return value reports whether anything was removed so the caller can
// warn (throttled) that the channel configuration should move the credential to
// the api_key field.
func sanitizeVideoTaskBaseURL(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		// Unparsable input: keep it verbatim rather than silently dropping the
		// routing information (it comes from channel configuration, not from a
		// request body).
		return trimmed, false
	}
	changed := false

	if parsed.User != nil {
		if parsed.User.Username() != "" {
			changed = true
		}
		if _, hasPassword := parsed.User.Password(); hasPassword {
			changed = true
		}
		parsed.User = nil
	}

	if parsed.RawQuery != "" {
		query := parsed.Query()
		for name := range query {
			if _, sensitive := videoTaskURLCredentialParams[strings.ToLower(name)]; sensitive {
				query.Del(name)
				changed = true
			}
		}
		if changed {
			parsed.RawQuery = query.Encode()
		}
	}

	return parsed.String(), changed
}

// SaveVideoTask persists the channel pin used to poll a video task.
//
// Two long-term hazards were fixed here: the stored base URL kept any inline
// credentials, and nothing ever deleted task rows (the table grew forever). The
// URL is stored without credentials, and the write path prunes old rows in a
// bounded batch at most once per hour (see retention_maintenance.go).
func (r *Repository) SaveVideoTask(ctx context.Context, task *VideoTask) error {
	if task == nil {
		return fmt.Errorf("video task is required")
	}
	baseURL, sanitized := sanitizeVideoTaskBaseURL(task.BaseURL)
	if sanitized {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "video-task-url-credentials", time.Hour,
			"video task base URL contained credentials; stored it without them",
			"channel", task.ChannelName,
			"remediation", "configure the credential as the channel api_key instead of embedding it in base_url",
		)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO video_tasks
		(id, upstream_id, owner, model, channel_id, channel_name, base_url) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.UpstreamID, task.Owner, task.Model, task.ChannelID, task.ChannelName, baseURL)
	if err != nil {
		return err
	}
	r.maybePruneVideoTasks()
	return nil
}

func (r *Repository) GetVideoTask(ctx context.Context, id, owner string) (*VideoTask, error) {
	var task VideoTask
	err := r.db.QueryRowContext(ctx, `SELECT id, upstream_id, owner, model, channel_id, channel_name, base_url
		FROM video_tasks WHERE id = ? AND owner = ?`, id, owner).Scan(
		&task.ID, &task.UpstreamID, &task.Owner, &task.Model, &task.ChannelID, &task.ChannelName, &task.BaseURL)
	return &task, err
}
