package storage

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
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
func (r *Repository) GetUserByUsername(username string) (*UserRecord, error) {
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(balance_units, 0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE username = ?`, username)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.BalanceUnits, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.resolveBalanceUnits()
	return &u, nil
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
func (r *Repository) GetUserByIDContext(ctx context.Context, id int64) (*UserRecord, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRowContext(ctx, `SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(balance_units, 0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE id = ?`, id)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.BalanceUnits, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.resolveBalanceUnits()
	return &u, nil
}

// GetUserByEmail finds a user by email address. The lookup is case-insensitive
// because the uniqueness guarantee is on LOWER(email); ORDER BY id keeps the
// result deterministic on databases where the unique index could not be created
// and duplicate rows still exist.
func (r *Repository) GetUserByEmail(email string) (*UserRecord, error) {
	if r == nil || r.db == nil || strings.TrimSpace(email) == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(balance_units, 0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE email <> '' AND LOWER(email) = LOWER(?) ORDER BY id ASC LIMIT 1`, strings.TrimSpace(email))
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.BalanceUnits, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.resolveBalanceUnits()
	return &u, nil
}

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
	// New accounts start in the root unit; an administrator can move them later
	// (the organization UI/API lands in M2). Reading the org in the same INSERT
	// keeps account creation atomic.
	id, err := db.InsertGetID(`INSERT INTO users (username, email, password_hash, role, status, balance, balance_units, group_name, org_id, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, (SELECT id FROM orgs WHERE code = 'default' LIMIT 1), CURRENT_TIMESTAMP)`,
		u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.Balance, ToUnits(u.Balance), u.GroupName)
	if err != nil {
		if isEmailConflictError(err) {
			return ErrEmailAlreadyRegistered
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

const MaxUserBalance = 1_000_000_000.0

var ErrBalanceOutOfRange = errors.New("账户余额或调整金额必须在 -1000000000 至 1000000000 元之间")

func validUserBalance(amount float64) bool {
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && math.Abs(amount) <= MaxUserBalance
}

func requireUserUpdated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

// UpdateUserBalance adjusts a user's wallet balance by a delta amount.
func (r *Repository) UpdateUserBalance(username string, delta float64) error {
	if !validUserBalance(delta) {
		return ErrBalanceOutOfRange
	}
	units := ToUnits(delta)
	limit := ToUnits(MaxUserBalance)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var userID, oldUnits int64
	var status string
	if err := tx.QueryRow(`SELECT id, COALESCE(status, 'active'), COALESCE(balance_units, 0) FROM users WHERE username = ?`, username).Scan(&userID, &status, &oldUnits); err != nil {
		return err
	}
	if status == "deleted" {
		return ErrBalanceOutOfRange
	}
	newUnits := oldUnits + units
	if newUnits < -limit || newUnits > limit {
		return ErrBalanceOutOfRange
	}
	if err := requireUserUpdated(tx.Exec(`UPDATE users SET balance = ?, balance_units = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'deleted'`, FromUnits(newUnits), newUnits, userID)); err != nil {
		return err
	}
	if err := r.appendWalletLedger(tx, userID, WalletTypeAdjustment, units, newUnits, "admin", "balance delta", ""); err != nil {
		return err
	}
	return tx.Commit()
}

// SetUserBalance sets an exact wallet balance for a user.
func (r *Repository) SetUserBalance(username string, balance float64) error {
	if !validUserBalance(balance) {
		return ErrBalanceOutOfRange
	}
	newUnits := ToUnits(balance)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var userID, oldUnits int64
	var status string
	if err := tx.QueryRow(`SELECT id, COALESCE(status, 'active'), COALESCE(balance_units, 0) FROM users WHERE username = ?`, username).Scan(&userID, &status, &oldUnits); err != nil {
		return err
	}
	if status == "deleted" {
		return sql.ErrNoRows
	}
	if err := requireUserUpdated(tx.Exec(`UPDATE users SET balance = ?, balance_units = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'deleted'`, balance, newUnits, userID)); err != nil {
		return err
	}
	if err := r.appendWalletLedger(tx, userID, WalletTypeAdjustment, newUnits-oldUnits, newUnits, "admin", "balance set", ""); err != nil {
		return err
	}
	return tx.Commit()
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

// DeductUserBalance deducts quota/cost from user wallet. Admins are exempt.
func (r *Repository) DeductUserBalance(userID int64, cost float64) error {
	if userID <= 0 || cost <= 0 {
		return nil
	}
	_, err := r.db.Exec(`UPDATE users SET balance = balance - ?, balance_units = COALESCE(balance_units, 0) - ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND role != 'admin'`, cost, ToUnits(cost), userID)
	return err
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
