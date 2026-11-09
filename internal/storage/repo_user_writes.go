package storage

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// CreateUser inserts a new user record.
func (r *Repository) CreateUser(u *UserRecord) error {
	return insertUser(r.db, u)
}

type recordInserter interface {
	InsertGetID(query string, args ...any) (int64, error)
}

// userInserter is the insert surface used by insertUser: the email conflict
// pre-check needs a QueryRow in addition to the shared InsertGetID.
type userInserter interface {
	recordInserter
	QueryRow(query string, args ...any) *sql.Row
}

// ErrEmailAlreadyRegistered is returned when creating an account would duplicate
// an existing email address (compared case-insensitively). Handlers map it to a
// client error instead of a generic failure.
var ErrEmailAlreadyRegistered = errors.New("email already registered")

// ErrUsernameAlreadyRegistered is returned when creating an account would
// duplicate an existing username ignoring case. Account identity is
// case-insensitive (see GetUserByUsername and migration 44), so "Alice" cannot be
// created next to "alice".
var ErrUsernameAlreadyRegistered = errors.New("username already registered")

// isEmailConflictError reports whether err is the unique violation raised by
// usersEmailUniqueIndex. Both drivers put the index name in the error text;
// PostgreSQL additionally reports the SQLSTATE code.
func isEmailConflictError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
		return strings.Contains(strings.ToLower(pqErr.Constraint), usersEmailUniqueIndex)
	}
	return strings.Contains(strings.ToLower(err.Error()), usersEmailUniqueIndex)
}

// isUsernameConflictError reports whether err is the unique violation raised by
// usersUsernameUniqueIndex (the case-insensitive index on username).
func isUsernameConflictError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
		return strings.Contains(strings.ToLower(pqErr.Constraint), usersUsernameUniqueIndex)
	}
	return strings.Contains(strings.ToLower(err.Error()), usersUsernameUniqueIndex)
}

func insertUser(db userInserter, u *UserRecord) error {
	if u.Role == "" {
		u.Role = "user"
	}
	if u.Status == "" {
		u.Status = "active"
	}
	if u.GroupName == "" {
		u.GroupName = "default"
	}
	// Reject a duplicate email before inserting. This covers the common
	// sequential case (and databases where the unique index could not be created
	// because they still hold duplicates); the partial unique index on
	// LOWER(email) is what makes the concurrent case race-proof, and the insert
	// error is classified below.
	if email := strings.TrimSpace(u.Email); email != "" {
		var taken int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email <> '' AND LOWER(email) = LOWER(?)`, email).Scan(&taken); err != nil {
			return err
		}
		if taken > 0 {
			return ErrEmailAlreadyRegistered
		}
	}
	// Same for the username, which is unique ignoring case (migration 44). The
	// pre-check keeps the common sequential path a clean client error even on a
	// database where the index is still deferred due to legacy duplicates.
	if username := strings.TrimSpace(u.Username); username != "" {
		var taken int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE LOWER(username) = LOWER(?)`, username).Scan(&taken); err != nil {
			return err
		}
		if taken > 0 {
			return ErrUsernameAlreadyRegistered
		}
	}
	id, err := db.InsertGetID(`INSERT INTO users (username, email, password_hash, role, status, balance, balance_units, group_name, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.Balance, ToUnits(u.Balance), u.GroupName)
	if err != nil {
		if isEmailConflictError(err) {
			return ErrEmailAlreadyRegistered
		}
		if isUsernameConflictError(err) {
			return ErrUsernameAlreadyRegistered
		}
		return err
	}
	u.ID = id
	return nil
}

// CreateUserWithAPIKey keeps onboarding atomic: a failed key insert must not
// leave behind an account that the customer cannot register again.
func (r *Repository) CreateUserWithAPIKey(user *UserRecord, key *APIKeyRecord) error {
	return r.CreateOAuthUserWithAPIKey("", "", user, key)
}

func (r *Repository) CreateOAuthUserWithAPIKey(provider, subject string, user *UserRecord, key *APIKeyRecord) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertUser(tx, user); err != nil {
		return err
	}
	if subject != "" {
		if _, err := tx.Exec(`INSERT INTO oauth_identities (provider, subject, user_id) VALUES (?, ?, ?)`, provider, subject, user.ID); err != nil {
			return err
		}
	}
	key.UserID = user.ID
	key.TenantID = user.Username
	if key.GroupName == "" {
		key.GroupName = user.GroupName
	}
	if err := insertAPIKey(tx, key); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateUserPassword updates the password hash for a user.
func (r *Repository) UpdateUserPassword(username, newHash string) error {
	return requireUserUpdated(r.db.Exec(`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ? AND status <> 'deleted'`, newHash, username))
}

// UpdateUserStatus updates user status ('active' or 'locked').
func (r *Repository) UpdateUserStatus(username, status string) error {
	return requireUserUpdated(r.db.Exec(`UPDATE users SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ? AND status <> 'deleted'`, status, username))
}

// UpdateUserGroup updates a user's pricing group.
func (r *Repository) UpdateUserGroup(username, groupName string) error {
	if groupName == "" {
		groupName = "default"
	}
	return requireUserUpdated(r.db.Exec(`UPDATE users SET group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ? AND status <> 'deleted'`, groupName, username))
}

// UpdateUserRole updates a user's system role ('admin' or 'user').
func (r *Repository) UpdateUserRole(username, role string) error {
	if role != "admin" && role != "user" {
		role = "user"
	}
	return requireUserUpdated(r.db.Exec(`UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ? AND status <> 'deleted'`, role, username))
}

// DeleteUser closes an account and revokes its credentials in one transaction.
// Retain account and key identities for historical charges and delayed payments.
func (r *Repository) DeleteUser(username string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	if err := tx.QueryRow(`SELECT id FROM users WHERE username = ? AND status <> 'deleted'`, username).Scan(&userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE user_id = ? OR (COALESCE(user_id, 0) = 0 AND tenant_id = ?)`, userID, username); err != nil {
		return err
	}
	if err := requireUserUpdated(tx.Exec(`UPDATE users SET status = 'deleted', password_hash = '', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'deleted'`, userID)); err != nil {
		return err
	}
	return tx.Commit()
}

// EnsureDefaultAdmin initializes the default admin user if it does not already exist.
func (r *Repository) EnsureDefaultAdmin(username, plainPass string) error {
	existing, _ := r.GetUserByUsername(username)
	if existing != nil {
		// Ensure admin has infinite balance flag and active status
		if existing.Balance < 999999 {
			_ = r.SetUserBalance(username, 9999999.0)
		}
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return r.CreateUser(&UserRecord{
		Username:     username,
		Email:        "admin@airoute.local",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "active",
		Balance:      9999999.0, // Admin has infinite quota
		GroupName:    "default",
	})
}
