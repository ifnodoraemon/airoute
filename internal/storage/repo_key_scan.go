package storage

import "encoding/json"

const apiKeyColumns = `id, public_id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), COALESCE(admin_disabled, FALSE), created_at, updated_at`

func scanAPIKey(row rowScanner) (*APIKeyRecord, error) {
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.PublicID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.AdminDisabled, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
	return &rec, nil
}
