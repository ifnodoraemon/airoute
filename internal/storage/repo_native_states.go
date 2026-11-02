package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNativeStateNotFound = errors.New("native state not found")

// NativeState binds opaque vendor state to its caller, credential and model.
// Public vendor IDs alone are never proof of ownership.
type NativeState struct {
	Namespace, ID, Owner, Identity, Model string
	Data                                  []byte
	Billing                               []byte
	ExpiresAt                             time.Time
}

func createNativeStates(db *DB) error {
	stamp := "DATETIME"
	if db.Dialect() == "postgres" {
		stamp = "TIMESTAMPTZ"
	}
	_, err := db.DB.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS native_states (
		namespace TEXT NOT NULL, id TEXT NOT NULL, owner TEXT NOT NULL,
		identity TEXT NOT NULL, model TEXT NOT NULL DEFAULT '', data TEXT NOT NULL, billing TEXT NOT NULL DEFAULT '', claimed BIGINT NOT NULL DEFAULT 0, polled_at BIGINT NOT NULL DEFAULT 0,
		expires_at %s NOT NULL, PRIMARY KEY(namespace,id,owner));
		CREATE INDEX IF NOT EXISTS idx_native_states_expiry ON native_states(expires_at);`, stamp))
	return err
}

func (r *Repository) SaveNativeState(ctx context.Context, state NativeState) error {
	if state.Namespace == "" || state.ID == "" || state.Owner == "" || state.Identity == "" {
		return errors.New("native state scope is required")
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO native_states(namespace,id,owner,identity,model,data,billing,expires_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(namespace,id,owner) DO UPDATE SET
		data=excluded.data, expires_at=excluded.expires_at
		WHERE native_states.identity=excluded.identity AND native_states.model=excluded.model`,
		state.Namespace, state.ID, state.Owner, state.Identity, state.Model, string(state.Data), string(state.Billing), r.dialectTime(state.ExpiresAt))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("native state belongs to another upstream or model")
	}
	return nil
}

func (r *Repository) LoadNativeState(ctx context.Context, namespace, id, owner string) (*NativeState, error) {
	var state NativeState
	var data, billing string
	err := r.db.QueryRowContext(ctx, `SELECT namespace,id,owner,identity,model,data,billing FROM native_states
		WHERE namespace=? AND id=? AND owner=? AND expires_at>?`, namespace, id, owner, r.dialectTime(time.Now())).Scan(
		&state.Namespace, &state.ID, &state.Owner, &state.Identity, &state.Model, &data, &billing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNativeStateNotFound
	}
	if err != nil {
		return nil, err
	}
	state.Data = []byte(data)
	state.Billing = []byte(billing)
	return &state, nil
}

func (r *Repository) ListNativeStates(ctx context.Context, namespace, owner string) ([]NativeState, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT namespace,id,owner,identity,model,data FROM native_states
		WHERE namespace=? AND owner=? AND expires_at>? ORDER BY id LIMIT 1000`, namespace, owner, r.dialectTime(time.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []NativeState
	for rows.Next() {
		var s NativeState
		var data string
		if err := rows.Scan(&s.Namespace, &s.ID, &s.Owner, &s.Identity, &s.Model, &data); err != nil {
			return nil, err
		}
		s.Data = []byte(data)
		states = append(states, s)
	}
	return states, rows.Err()
}

func (r *Repository) DeleteNativeState(ctx context.Context, namespace, id, owner string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM native_states WHERE namespace=? AND id=? AND owner=?`, namespace, id, owner)
	return err
}

func (r *Repository) PruneNativeStates(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM native_states WHERE (namespace,id,owner) IN (
		SELECT namespace,id,owner FROM native_states WHERE expires_at<? LIMIT 500)`, r.dialectTime(time.Now()))
	return err
}

func (r *Repository) ClaimNativeState(ctx context.Context, namespace, id, owner string) (bool, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE native_states SET claimed=? WHERE namespace=? AND id=? AND owner=? AND (claimed=0 OR (claimed>1 AND claimed<?)) AND expires_at>?`, now.Add(time.Minute).Unix(), namespace, id, owner, now.Unix(), r.dialectTime(now))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *Repository) CompleteNativeState(ctx context.Context, namespace, id, owner string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE native_states SET claimed=1 WHERE namespace=? AND id=? AND owner=?`, namespace, id, owner)
	return err
}

func (r *Repository) PendingNativeJobs(ctx context.Context) ([]NativeState, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT namespace,id,owner,identity,model,data,billing FROM native_states
		WHERE namespace IN ('gemini-batch','gemini-interaction') AND claimed<>1 AND billing<>'' AND expires_at>? ORDER BY polled_at,expires_at LIMIT 100`, r.dialectTime(time.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NativeState
	for rows.Next() {
		var s NativeState
		var data, billing string
		if err := rows.Scan(&s.Namespace, &s.ID, &s.Owner, &s.Identity, &s.Model, &data, &billing); err != nil {
			return nil, err
		}
		s.Data = []byte(data)
		s.Billing = []byte(billing)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) TouchNativeJob(ctx context.Context, s NativeState) error {
	_, err := r.db.ExecContext(ctx, `UPDATE native_states SET polled_at=? WHERE namespace=? AND id=? AND owner=?`, time.Now().Unix(), s.Namespace, s.ID, s.Owner)
	return err
}
