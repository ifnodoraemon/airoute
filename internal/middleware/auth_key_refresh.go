package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Refresh on every request: snapshot lag must not conceal a disabled key.
// A key just read from storage is already authoritative and needs no second query.
func refreshAuthKey(c *gin.Context, key *model.APIKeyConfig, repo *storage.Repository, resolvedFromStorage, freeRead bool) bool {
	if repo == nil || resolvedFromStorage {
		return true
	}
	lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
	keyRec, lookupErr := repo.GetAPIKeyByKeyContext(lookupCtx, key.Key)
	cancel()
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		// The authoritative refresh failed (DB stall/timeout). The in-memory
		// snapshot may still list a just-revoked key as active, so fail closed
		// for billable requests, matching the account admission posture. Free
		// reads never settle a charge and stay available during a stall.
		if !freeRead {
			return abortResolvedKeyDenial(c, key.Key, &AccessDenial{
				StatusCode: http.StatusServiceUnavailable,
				Message:    "Key status is temporarily unavailable. Please retry shortly.",
				Type:       "service_unavailable",
				Code:       "key_status_unavailable",
				Headers:    map[string]string{"Retry-After": "2"},
			})
		}
		return true
	}
	if keyRec == nil {
		return true
	}
	if !abortResolvedKeyDenial(c, key.Key, KeyStatusDenial(keyRec.Status)) {
		return false
	}
	effectiveBudget := keyRec.Budget
	if effectiveBudget <= 0 {
		effectiveBudget = key.Budget
	}
	budgetUnits := keyRec.BudgetUnits
	if budgetUnits <= 0 {
		budgetUnits = key.BudgetUnits
	}
	if denial := KeyBudgetDenial(budgetUnits, keyRec.UsedCostUnits, effectiveBudget, keyRec.UsedCost); denial != nil && !freeRead {
		return abortResolvedKeyDenial(c, key.Key, denial)
	}
	if keyRec.GroupName != "" {
		key.GroupName = keyRec.GroupName
	}
	if keyRec.UserID > 0 {
		key.UserID = keyRec.UserID
	}
	if keyRec.ID > 0 {
		key.ID = keyRec.ID
	}
	if keyRec.UserStatus != "" {
		key.UserStatus = keyRec.UserStatus
		key.UserRole = keyRec.UserRole
		key.UserBalance = keyRec.UserBalance
	}
	return true
}
