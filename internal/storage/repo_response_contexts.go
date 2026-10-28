package storage

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// ResponseContextTTL bounds how long a Responses API conversation stays
// resumable through previous_response_id. A week covers normal agent sessions
// while stopping the table from growing forever: the upstream/provider
// conversation itself is stateless, so nothing else can reuse the data.
const ResponseContextTTL = 7 * 24 * time.Hour

const (
	// Bound each DELETE so a long-neglected table cannot hold a write lock for a
	// long time on the SQLite deployment.
	responseContextPruneBatchSize = 500
	// At most one prune attempt per interval.
	responseContextPruneInterval = time.Hour
)

// ErrResponseContextNotFound is returned when a previous_response_id does not
// resolve to a stored conversation: the row is missing, expired, or owned by a
// different caller. All three cases are indistinguishable on purpose, so a
// client cannot probe for another tenant's response ids.
var ErrResponseContextNotFound = errors.New("response context not found")

// responseContextPruneAt throttles opportunistic pruning. It is package-level
// rather than a Repository field because the write path only needs "don't prune
// more than once an hour", and there is one long-lived repository per process.
var responseContextPruneAt atomic.Int64

// ResponseContext is the persisted conversation behind one Responses response id.
type ResponseContext struct {
	ID    string
	Owner string
	// Messages is the JSON-encoded []model.ChatMessage conversation. The storage
	// package keeps it opaque so it does not depend on the wire model.
	Messages []byte
}

// SaveResponseContext stores the conversation of a completed response under its
// client-facing response id.
func (r *Repository) SaveResponseContext(ctx context.Context, id, owner string, messages []byte) error {
	if r == nil || r.db == nil {
		return errors.New("response context storage is unavailable")
	}
	if id == "" || owner == "" {
		return errors.New("response context id and owner are required")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO response_contexts
		(id, owner, messages, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, owner, string(messages),
		r.dialectTime(now),
		r.dialectTime(now.Add(ResponseContextTTL)),
	)
	if err != nil {
		return err
	}
	r.maybePruneResponseContexts()
	return nil
}

// LoadResponseContext returns the conversation stored for id when it belongs to
// owner and has not expired yet.
func (r *Repository) LoadResponseContext(ctx context.Context, id, owner string) (*ResponseContext, error) {
	if r == nil || r.db == nil {
		return nil, ErrResponseContextNotFound
	}
	if id == "" || owner == "" {
		return nil, ErrResponseContextNotFound
	}
	var stored ResponseContext
	var messages string
	err := r.db.QueryRowContext(ctx, `SELECT id, owner, messages FROM response_contexts
		WHERE id = ? AND owner = ? AND expires_at > ?`,
		id, owner, r.dialectTime(time.Now()),
	).Scan(&stored.ID, &stored.Owner, &messages)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrResponseContextNotFound
	}
	if err != nil {
		return nil, err
	}
	stored.Messages = []byte(messages)
	return &stored, nil
}

// PruneResponseContexts deletes contexts that expired before cutoff, in bounded
// batches.
func (r *Repository) PruneResponseContexts(cutoff time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	cutoffValue := r.dialectTime(cutoff)
	var total int64
	for {
		res, err := r.db.Exec(
			`DELETE FROM response_contexts WHERE id IN (
				SELECT id FROM response_contexts WHERE expires_at < ? ORDER BY expires_at LIMIT ?)`,
			cutoffValue, responseContextPruneBatchSize,
		)
		if err != nil {
			return total, err
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < responseContextPruneBatchSize {
			return total, nil
		}
	}
}

// maybePruneResponseContexts prunes expired response contexts from the write
// path, at most once per interval.
func (r *Repository) maybePruneResponseContexts() {
	if r == nil || r.db == nil {
		return
	}
	if !shouldRunRetention(&responseContextPruneAt, responseContextPruneInterval) {
		return
	}
	if n, err := r.PruneResponseContexts(time.Now().UTC()); err != nil {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "response-context-prune-failed", 10*time.Minute,
			"failed to prune expired responses contexts", "error", err.Error())
	} else if n > 0 {
		telemetry.ScopedLogger("system").Info("pruned expired responses contexts", "deleted", n)
	}
}
