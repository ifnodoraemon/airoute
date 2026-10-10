package storage

import (
	"strings"
	"time"
)

// VerificationCodeRecord stores temporary email verification codes.
type VerificationCodeRecord struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// SaveVerificationCode stores a verification code with an expiration window.
func (r *Repository) SaveVerificationCode(email, code, purpose string, duration time.Duration) error {
	expiresAt := time.Now().Add(duration)
	_, err := r.db.Exec(`INSERT INTO verification_codes (email, code, purpose, expires_at, used) VALUES (?, ?, ?, ?, 0)`,
		strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(code), purpose, expiresAt.UTC().Format("2006-01-02 15:04:05"))
	return err
}

// VerifyCode validates and consumes a verification code.
func (r *Repository) VerifyCode(email, code, purpose string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return false
	}

	var id int64
	// Expiry comparison: SQLite normalizes via datetime(); Postgres compares
	// natively with NOW(). Rebind's string translation covers neither the
	// expires_at column nor the 'now' literal, so the branch is explicit.
	codeQuery := `SELECT id FROM verification_codes WHERE email = ? AND code = ? AND purpose = ? AND used = 0 AND datetime(expires_at) > datetime('now') ORDER BY id DESC LIMIT 1`
	if r.db.Dialect() == "postgres" {
		codeQuery = `SELECT id FROM verification_codes WHERE email = ? AND code = ? AND purpose = ? AND used = 0 AND expires_at > NOW() ORDER BY id DESC LIMIT 1`
	}
	row := r.db.QueryRow(codeQuery, email, code, purpose)
	if err := row.Scan(&id); err != nil {
		return false
	}

	// Mark as used
	_, _ = r.db.Exec(`UPDATE verification_codes SET used = 1 WHERE id = ?`, id)
	return true
}
