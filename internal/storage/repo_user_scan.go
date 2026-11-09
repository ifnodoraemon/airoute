package storage

const userColumns = `id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(balance_units, 0), COALESCE(group_name, 'default'), created_at, updated_at`

func scanUser(row rowScanner) (*UserRecord, error) {
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.BalanceUnits, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.resolveBalanceUnits()
	return &u, nil
}
