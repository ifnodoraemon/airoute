package storage

import "strings"

// All participants lock keys first, then wallets by ID. A batch must acquire
// every key before it takes any wallet: otherwise admission for key B can wait
// on a wallet held by a batch that subsequently needs B.
func (r *Repository) lockSettlementAccounts(tx *Tx, logs []*UsageLogRecord) error {
	if r.db.Dialect() != "postgres" {
		return nil
	}
	keys, tenants := map[string]bool{}, map[string]bool{}
	for _, rec := range logs {
		if rec.APIKey != "" {
			keys[rec.APIKey] = true
		}
		if rec.TenantID != "" {
			tenants[rec.TenantID] = true
		}
	}
	keyArgs, tenantArgs := []any{}, []any{}
	for _, key := range sortedMapKeys(keys) {
		keyArgs = append(keyArgs, key)
	}
	for _, tenant := range sortedMapKeys(tenants) {
		tenantArgs = append(tenantArgs, tenant)
	}
	conditions, args := []string{}, []any{}
	if len(keyArgs) > 0 {
		in := strings.TrimSuffix(strings.Repeat("?,", len(keyArgs)), ",")
		if err := consumeLockedRows(tx, `SELECT key FROM api_keys WHERE key IN (`+in+`) ORDER BY key FOR UPDATE`, keyArgs); err != nil {
			return err
		}
		conditions = append(conditions, `id IN (SELECT user_id FROM api_keys WHERE key IN (`+in+`))`)
		args = append(args, keyArgs...)
	}
	if len(tenantArgs) > 0 {
		in := strings.TrimSuffix(strings.Repeat("?,", len(tenantArgs)), ",")
		conditions = append(conditions, `username IN (`+in+`) OR email IN (`+in+`)`)
		args = append(args, tenantArgs...)
		args = append(args, tenantArgs...)
	}
	if len(conditions) == 0 {
		return nil
	}
	return consumeLockedRows(tx, `SELECT id FROM users WHERE `+strings.Join(conditions, " OR ")+` ORDER BY id FOR UPDATE`, args)
}

func consumeLockedRows(tx *Tx, query string, args []any) error {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		// Consume every row so every lock is acquired before the next stage.
	}
	return rows.Err()
}

// The debit and its hold disappear in the same commit. A failed transaction
// restores both; redelivery removes only the reservation named on that record.
func finishBillingReservations(tx *Tx, logs []*UsageLogRecord) error {
	ids := map[string]string{}
	for _, rec := range logs {
		if rec != nil && rec.ReservationID != "" {
			ids[rec.ReservationID] = NormalizeStoredAPIKeyReference(rec.APIKey)
		}
	}
	for _, id := range sortedMapKeys(ids) {
		if _, err := tx.Exec(`DELETE FROM billing_reservations WHERE id = ? AND api_key = ?`, id, ids[id]); err != nil {
			return err
		}
	}
	return nil
}
