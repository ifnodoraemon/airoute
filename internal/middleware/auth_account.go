package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// shouldFailClosedOnUnknownAccount decides the admission posture when the key
// belongs to a user but the gateway cannot establish that user's status or
// balance (snapshot built without user fields + authoritative lookup timed out).
//
// The old behaviour was to admit: the request skipped the wallet gate and the
// pre-authorisation, which is exactly the burst the amount reservation exists to
// stop. Free reads are exempt — they never settle a charge, and blocking a
// job-status poll during a database stall protects no money.
func shouldFailClosedOnUnknownAccount(freeRead bool, userStatus string, userID int64) bool {
	return !freeRead && userStatus == "" && userID > 0
}

type authAccount struct {
	status   string
	role     string
	balance  float64
	userID   int64
	budgeted bool
}

func resolveAuthAccount(c *gin.Context, key *model.APIKeyConfig, repo *storage.Repository) authAccount {
	account := authAccount{status: key.UserStatus, role: key.UserRole, balance: key.UserBalance, userID: key.UserID}
	account.budgeted = key.BudgetUnits > 0 || key.Budget > 0
	if account.status == "" && key.UserID > 0 && repo != nil {
		lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
		u, _ := repo.GetUserByIDContext(lookupCtx, key.UserID)
		cancel()
		if u != nil {
			account.status, account.role, account.balance = u.Status, u.Role, u.Balance
		}
	}
	return account
}

func admitAuthAccount(c *gin.Context, digest string, account authAccount, freeRead bool) bool {
	if shouldFailClosedOnUnknownAccount(freeRead, account.status, account.userID) {
		denial := BalanceUnknownDenial()
		auditResolvedKeyFailure(c, digest, denial.StatusCode, denial.LogMessage)
		AbortWithDenial(c, denial)
		return false
	}
	if freeRead {
		return abortResolvedKeyDenial(c, digest, AccountLockedDenial(account.status))
	}
	return abortResolvedKeyDenial(c, digest, AccountDenial(account.status, account.role, account.balance))
}

// The returned cleanup stays deferred around c.Next, after downstream settlement.
func reserveAuthQuota(c *gin.Context, digest string, account authAccount, freeRead bool) (func(), bool) {
	if freeRead || (c.Request.Method == http.MethodGet && !strings.EqualFold(c.GetHeader("Upgrade"), "websocket")) || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
		return nil, true
	}
	repo := storage.GetGlobalRepository()
	if repo == nil && account.userID <= 0 && !account.budgeted {
		return nil, true
	}
	quota, err := AcquireRequestQuota(c.Request.Context(), repo, digest)
	if err != nil {
		return nil, abortResolvedKeyDenial(c, digest, QuotaErrorDenial(err))
	}
	if quota == nil {
		return nil, true
	}
	c.Set(contextKeyRequestQuota, quota)
	return quota.Close, true
}
