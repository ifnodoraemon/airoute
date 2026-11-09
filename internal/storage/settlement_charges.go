package storage

// fallbackChargeToTenant routes an unapplied key charge the way the settlement
// rules require: an admin-owned key is exempt, a named tenant absorbs the charge
// on its own wallet, and anything else is reported as uncollected.
func fallbackChargeToTenant(key, tenant string, cost float64, ownerIsAdmin bool, orphanTenantCost map[string]float64) {
	if ownerIsAdmin {
		// Admin keys are exempt; do not fallback to tenant.
		return
	}
	if tenant != "" {
		orphanTenantCost[tenant] += cost
		return
	}
	// No owner row and no tenant to fall back to: the ledger row that was already
	// written records the charge as settled, but no wallet moved. Keep the two in
	// agreement (the delivery must not be retried) and make the uncollected money
	// visible instead of dropping it silently.
	reportUnattributedCharge(key, "", cost)
}

// applyChargeAggregates updates key quota counters and deducts the charged amount
// from the owning wallet (with the tenant fallback and admin exemption rules).
//
// The map keys are always traversed in sorted order so concurrent batches take the
// same locks in the same order.
func (r *Repository) applyChargeAggregates(tx *Tx, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64, effects *settlementEffects) error {
	keys := sortedMapKeys(keyCost)
	if len(keys) > 0 {
		var err error
		if r.db.Dialect() == "postgres" {
			err = r.applyPostgresChargeMerges(tx, keys, keyCost, keyTokens, keyTenant, orphanTenantCost, effects)
		} else {
			err = r.applyPortableChargeUpdates(tx, keys, keyCost, keyTokens, keyTenant, orphanTenantCost, effects)
		}
		if err != nil {
			return err
		}
	}

	for _, tenant := range sortedMapKeys(orphanTenantCost) {
		cost := orphanTenantCost[tenant]
		if cost <= 0 {
			continue
		}
		applied, _, err := r.deductWallet(tx,
			`id = (SELECT id FROM users WHERE username = ? OR email = ? ORDER BY CASE WHEN username = ? THEN 0 ELSE 1 END, id LIMIT 1) AND role != 'admin'`, []any{tenant, tenant, tenant}, cost, effects)
		if err != nil {
			return err
		}
		if !applied {
			// The tenant named on the record no longer matches any billable user.
			reportUnattributedCharge("", tenant, cost)
		}
	}
	return nil
}

// applyPortableChargeUpdates is the SQLite path: one UPDATE api_keys per key, then
// one exact UPDATE users per key.
//
// SQLite keeps the two statements: it has a single writer, so the extra round trip
// does not change concurrency, and its UPDATE ... FROM support is recent enough
// that folding the counter into the wallet statement would add a version
// dependency for no gain.
func (r *Repository) applyPortableChargeUpdates(tx *Tx, keys []string, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64, effects *settlementEffects) error {
	updateKeyStmt, err := tx.Prepare(r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_cost_units = COALESCE(used_cost_units, 0) + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`))
	if err != nil {
		return err
	}
	defer updateKeyStmt.Close()
	for _, key := range keys {
		cost, tokens := keyCost[key], keyTokens[key]
		if cost <= 0 && tokens <= 0 {
			continue
		}
		if _, err := updateKeyStmt.Exec(cost, ToUnits(cost), tokens, key); err != nil {
			return err
		}
	}

	for _, key := range keys {
		cost := keyCost[key]
		if cost <= 0 {
			continue
		}
		applied, _, err := r.deductWallet(tx,
			`id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`, []any{key}, cost, effects)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		var ownerIsAdmin bool
		row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), key)
		if err := row.Scan(&ownerIsAdmin); err == nil && ownerIsAdmin {
			// Admin keys are exempt; do not fallback to tenant.
			continue
		}
		fallbackChargeToTenant(key, keyTenant[key], cost, false, orphanTenantCost)
	}
	return nil
}
