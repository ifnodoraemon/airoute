package storage

import (
	"database/sql"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// UserRecord represents an administrator, operator, or standard user account.
type UserRecord struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`       // "admin" or "user"
	Status       string    `json:"status"`     // "active" or "locked"
	Balance      float64   `json:"balance"`    // Wallet balance in CNY
	GroupName    string    `json:"group_name"` // "default", "vip", "enterprise"
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetUserByUsername finds a user by username or email.
func (r *Repository) GetUserByUsername(username string) (*UserRecord, error) {
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE username = ? OR email = ?`, username, username)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID finds a user by primary key ID.
func (r *Repository) GetUserByID(id int64) (*UserRecord, error) {
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE id = ?`, id)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByEmail finds a user by email address.
func (r *Repository) GetUserByEmail(email string) (*UserRecord, error) {
	if r == nil || r.db == nil || strings.TrimSpace(email) == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE email = ?`, strings.TrimSpace(email))
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser inserts a new user record.
func (r *Repository) CreateUser(u *UserRecord) error {
	if u.Role == "" {
		u.Role = "user"
	}
	if u.Status == "" {
		u.Status = "active"
	}
	if u.GroupName == "" {
		u.GroupName = "default"
	}
	id, err := r.db.InsertGetID(`INSERT INTO users (username, email, password_hash, role, status, balance, group_name, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.Balance, u.GroupName)
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

// UpdateUserPassword updates the password hash for a user.
func (r *Repository) UpdateUserPassword(username, newHash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, newHash, username)
	return err
}

// UpdateUserStatus updates user status ('active' or 'locked').
func (r *Repository) UpdateUserStatus(username, status string) error {
	_, err := r.db.Exec(`UPDATE users SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, status, username)
	return err
}

// UpdateUserBalance adjusts a user's wallet balance by a delta amount.
func (r *Repository) UpdateUserBalance(username string, delta float64) error {
	_, err := r.db.Exec(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, delta, username)
	return err
}

// SetUserBalance sets an exact wallet balance for a user.
func (r *Repository) SetUserBalance(username string, balance float64) error {
	_, err := r.db.Exec(`UPDATE users SET balance = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, balance, username)
	return err
}

// UpdateUserGroup updates a user's pricing group.
func (r *Repository) UpdateUserGroup(username, groupName string) error {
	if groupName == "" {
		groupName = "default"
	}
	_, err := r.db.Exec(`UPDATE users SET group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, groupName, username)
	return err
}

// UpdateUserRole updates a user's system role ('admin' or 'user').
func (r *Repository) UpdateUserRole(username, role string) error {
	if role != "admin" && role != "user" {
		role = "user"
	}
	_, err := r.db.Exec(`UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, role, username)
	return err
}

// DeductUserBalance deducts quota/cost from user wallet. Admins are exempt.
func (r *Repository) DeductUserBalance(userID int64, cost float64) error {
	if userID <= 0 || cost <= 0 {
		return nil
	}
	_, err := r.db.Exec(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND role != 'admin'`, cost, userID)
	return err
}

// ListUsers returns all registered users without password hashes.
func (r *Repository) ListUsers() ([]*UserRecord, error) {
	rows, err := r.db.Query(`SELECT id, username, COALESCE(email, ''), COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*UserRecord
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &u)
	}
	return list, nil
}

// DeleteUser removes a user by username and cleans up associated API keys.
func (r *Repository) DeleteUser(username string) error {
	u, _ := r.GetUserByUsername(username)
	if u != nil && u.ID > 0 {
		_, _ = r.db.Exec(`DELETE FROM api_keys WHERE user_id = ?`, u.ID)
	}
	_, err := r.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	return err
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
