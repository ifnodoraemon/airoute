package storage

// postgresChargeMergeSQL folds the three statements the settlement path used to
// issue per key — UPDATE api_keys counters, UPDATE users wallet, and (on refusal)
// SELECT users admin flag — into a single round trip.
//
// Equivalence with the portable path, piece by piece:
//   - key_update is exactly the old counter UPDATE (same columns, same expression).
//   - wallet_update is exactly deductWallet's users UPDATE: the same integer subtraction, the same float mirror derived from the integer column, and the same
//     `role <> 'admin'` exemption. `AND ? > 0` reproduces deductWallet's early
//     return for a charge that rounds to zero units (it never touched a wallet and
//     fell through to the tenant fallback).
//   - owner_is_admin reproduces the old "SELECT (role = 'admin')" probe that only
//     ran after a refusal; NULL/absent users scan as 0, exactly like the discarded
//     scan error did before.
//   - the count columns distinguish "key row missing" from "wallet refused" without
//     another query, and balance_units feeds the overdraft accounting.
//
// The caller first locks all affected keys, then all affected wallets, in sorted
// order. Admission follows that key-to-wallet order as well.
//
// Every placeholder is cast explicitly. lib/pq sends parameters as untyped text, so
// the server infers their types from context, and a bare `? > 0` infers *integer* —
// which overflows on a 2.5-CNY charge (2.5e9 units) with a confusing 22003 error
// instead of a settlement. The casts pin the intended types.
const postgresChargeMergeSQL = `
WITH key_update AS (
	UPDATE api_keys
	   SET used_cost = used_cost + CAST(? AS DOUBLE PRECISION),
	       used_cost_units = COALESCE(used_cost_units, 0) + CAST(? AS BIGINT),
	       used_tokens = used_tokens + CAST(? AS BIGINT),
	       updated_at = CURRENT_TIMESTAMP
	 WHERE key = CAST(? AS TEXT)
	 RETURNING user_id
), wallet_update AS (
	UPDATE users u
	   SET balance_units = COALESCE(u.balance_units, 0) - CAST(? AS BIGINT),
	       balance = (COALESCE(u.balance_units, 0) - CAST(? AS BIGINT)) * 1.0 / 1000000000.0,
	       updated_at = CURRENT_TIMESTAMP
	  FROM key_update k
	 WHERE u.id = k.user_id
	   AND u.role <> 'admin'
	   AND CAST(? AS BIGINT) > 0
	 RETURNING u.balance_units
)
SELECT (SELECT COUNT(*) FROM key_update) AS key_rows,
       (SELECT COUNT(*) FROM wallet_update) AS wallet_rows,
       COALESCE((SELECT balance_units FROM wallet_update LIMIT 1), 0) AS balance_units,
       COALESCE((SELECT CASE WHEN u.role = 'admin' THEN 1 ELSE 0 END
                   FROM users u
                  WHERE u.id = (SELECT user_id FROM key_update)), 0) AS owner_is_admin`

// applyPostgresChargeMerges runs the merged statement once per key, in the sorted
// order the caller provides.
func (r *Repository) applyPostgresChargeMerges(tx *Tx, keys []string, keyCost map[string]float64, keyTokens map[string]int64, keyTenant map[string]string, orphanTenantCost map[string]float64, effects *settlementEffects) error {
	stmt, err := tx.Prepare(r.db.Rebind(postgresChargeMergeSQL))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, key := range keys {
		cost, tokens := keyCost[key], keyTokens[key]
		if cost <= 0 && tokens <= 0 {
			continue
		}
		units := ToUnits(cost)
		args := []any{cost, units, tokens, key, units, units, units}
		var keyRows, walletRows, balanceUnits, ownerIsAdmin int64
		if err := stmt.QueryRow(args...).Scan(&keyRows, &walletRows, &balanceUnits, &ownerIsAdmin); err != nil {
			return err
		}
		if walletRows > 0 {
			recordWalletDeductionEffect(balanceUnits, effects)
			continue
		}
		if cost <= 0 {
			// Counter-only key: the wallet was never asked to move.
			continue
		}
		fallbackChargeToTenant(key, keyTenant[key], cost, ownerIsAdmin == 1, orphanTenantCost)
	}
	return nil
}
