package storage

// OAuth identities are independent of mutable profile names and email addresses.
func (r *Repository) GetUserByOAuthIdentity(provider, subject string) (*UserRecord, error) {
	var id int64
	if err := r.db.QueryRow(`SELECT user_id FROM oauth_identities WHERE provider = ? AND subject = ?`, provider, subject).Scan(&id); err != nil {
		return nil, err
	}
	return r.GetUserByID(id)
}

func createOAuthIdentities(db *DB) error {
	_, err := db.DB.Exec(`CREATE TABLE IF NOT EXISTS oauth_identities (
		provider TEXT NOT NULL,
		subject TEXT NOT NULL,
		user_id BIGINT NOT NULL REFERENCES users(id),
		PRIMARY KEY (provider, subject)
	)`)
	return err
}
