package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// UserRecord represents an administrator, operator, or standard user account.
type UserRecord struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`          // "admin" or "user"
	Status       string    `json:"status"`        // "active" or "locked"
	Balance      float64   `json:"balance"`       // Wallet balance in CNY (legacy float mirror)
	BalanceUnits int64     `json:"balance_units"` // Wallet balance in 1e-9 CNY (authoritative integer ledger)
	GroupName    string    `json:"group_name"`    // "default", "vip", "enterprise"
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// resolveBalanceUnits prefers the integer ledger when it carries a non-zero value, or
// when the legacy float and the mirror disagree because the mirror was updated
// more recently.
func (u *UserRecord) resolveBalanceUnits() {
	if u == nil {
		return
	}
	if u.BalanceUnits != 0 || u.Balance == 0 {
		u.Balance = FromUnits(u.BalanceUnits)
		return
	}
	u.BalanceUnits = ToUnits(u.Balance)
}

// GetUserByUsername resolves a canonical account identity, never an email alias.
// Deleted identities remain reserved so old tokens and payment orders cannot
// become attached to a newly registered account with the same username.
//
// The comparison ignores case, because account identity is case-insensitive
// everywhere else: the login throttle lower-cases the username before building
// its lockout bucket (controlplane.login_guard), and allowing "Admin" and "admin"
// to be two accounts let an attacker own the case variant of a victim's name and
// reset the victim's failure counter by logging in (BUG-CP-02). Migration 44
// enforces the same rule as a unique index on LOWER(username); ORDER BY id keeps
// the result deterministic on databases where that index is still deferred
// because duplicate rows exist.
func (r *Repository) GetUserByUsername(username string) (*UserRecord, error) {
	row := r.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE LOWER(username) = LOWER(?) ORDER BY id ASC LIMIT 1`, strings.TrimSpace(username))
	return scanUser(row)
}

// GetUserByLogin allows email sign-in only at the login boundary. An exact
// username takes precedence when legacy accounts have overlapping identifiers.
func (r *Repository) GetUserByLogin(identifier string) (*UserRecord, error) {
	identifier = strings.TrimSpace(identifier)
	u, err := r.GetUserByUsername(identifier)
	if errors.Is(err, sql.ErrNoRows) {
		return r.GetUserByEmail(strings.ToLower(identifier))
	}
	return u, err
}

// GetUserByID finds a user by primary key ID.
func (r *Repository) GetUserByID(id int64) (*UserRecord, error) {
	return r.GetUserByIDContext(context.Background(), id)
}

// GetUserByIDContext is GetUserByID bound to a caller context/deadline.
//
// It reads through Reader(): this is the per-request balance/auth source on the
// data plane, and on SQLite it must not queue behind the single settlement writer.
// The read pool only ever sees committed transactions, so it cannot observe a
// half-applied wallet movement.
func (r *Repository) GetUserByIDContext(ctx context.Context, id int64) (*UserRecord, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrNoRows
	}
	row := r.db.Reader().QueryRowContext(ctx, r.db.Rebind(`SELECT `+userColumns+` FROM users WHERE id = ?`), id)
	return scanUser(row)
}

// GetUserByEmail finds a user by email address. The lookup is case-insensitive
// because the uniqueness guarantee is on LOWER(email); ORDER BY id keeps the
// result deterministic on databases where the unique index could not be created
// and duplicate rows still exist.
func (r *Repository) GetUserByEmail(email string) (*UserRecord, error) {
	if r == nil || r.db == nil || strings.TrimSpace(email) == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE email <> '' AND LOWER(email) = LOWER(?) ORDER BY id ASC LIMIT 1`, strings.TrimSpace(email))
	return scanUser(row)
}

// ListUsers returns all registered users without password hashes.
func (r *Repository) ListUsers() ([]*UserRecord, error) {
	rows, err := r.db.Query(`SELECT id, username, COALESCE(email, ''), COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(balance_units, 0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE status <> 'deleted' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*UserRecord
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.Status, &u.Balance, &u.BalanceUnits, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.resolveBalanceUnits()
		list = append(list, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}
