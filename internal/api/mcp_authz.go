package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// contextKeyCallerRecord binds the fully authorised caller to a JSON-RPC
// request. Tools reuse it instead of re-deriving the identity from the key, so
// the transport's admission decision cannot be bypassed by a tool handler.
const contextKeyCallerRecord mcpContextKey = "mcp_caller_record"

// mcpCaller is the authorised caller of an MCP request.
type mcpCaller struct {
	Key    string
	Record *storage.APIKeyRecord
	Config *model.APIKeyConfig
}

// mcpAccessError is a client-visible MCP rejection: the HTTP status to return
// plus a JSON-RPC error code/message.
type mcpAccessError struct {
	StatusCode int
	Code       int
	Message    string
}

// resolveCaller authenticates an MCP caller and applies the data-plane
// admission rules HTTP /v1/* enforces in middleware.AuthMiddleware: active key,
// existing owner, account lock, exhausted key budget and empty wallet.
//
// Before this, MCP only checked "key exists and status == active", so a locked
// or insolvent tenant kept consuming billable inference (A02/A04).
func (h *MCPHandler) resolveCaller(secret string) (*mcpCaller, *mcpAccessError) {
	// A caller-supplied credential is cleartext; normalize it to the stored digest.
	return h.resolveCallerDigest(storage.HashAPIKey(secret), secret)
}

// resolveCallerDigest resolves a caller whose key is already the stored digest
// (the SSE session path stores caller.Key, which is the digest). Hashing it again
// would never match, so the two entry points are kept separate instead of making
// the lookup guess which form it received - guessing would also let a digest be
// replayed as a bearer credential.
func (h *MCPHandler) resolveCallerDigest(key string, rawKey ...string) (*mcpCaller, *mcpAccessError) {
	// Preserve the historical message for a caller that supplied no key at all.
	if h.repo != nil && h.repo.CountAPIKeys() > 0 && strings.TrimSpace(key) == "" {
		return nil, &mcpAccessError{
			StatusCode: http.StatusUnauthorized,
			Code:       -32000,
			Message:    "Unauthorized: an API key is required for MCP access",
		}
	}

	rec, err := h.authenticateCallerDigest(key)
	if err != nil {
		return nil, &mcpAccessError{
			StatusCode: http.StatusUnauthorized,
			Code:       -32000,
			Message:    "Unauthorized: " + err.Error(),
		}
	}
	if rec == nil {
		// Open dev mode: no key has ever been registered.
		return &mcpCaller{Key: key}, nil
	}

	// The key's owner must still exist. The storage read joins users, so an
	// empty UserStatus on an owned key means the row is gone (deleted account).
	if rec.UserID > 0 && strings.TrimSpace(rec.UserStatus) == "" {
		return nil, mcpDataPlaneDenial(&middleware.AccessDenial{
			StatusCode: http.StatusForbidden,
			Message:    "User account has been deleted or is unavailable. Please contact administrator.",
			Type:       "account_locked",
			Code:       "account_locked",
		})
	}

	effective := mcpKeyConfigFromRecord(rec)
	if denial := middleware.AuthorizeKey(effective); denial != nil {
		return nil, mcpDataPlaneDenial(denial)
	}
	return &mcpCaller{Key: rec.Key, Record: rec, Config: effective}, nil
}

// mcpKeyConfigFromRecord adapts an authoritative key record to the config shape
// the shared data-plane rules operate on.
func mcpKeyConfigFromRecord(rec *storage.APIKeyRecord) *model.APIKeyConfig {
	if rec == nil {
		return nil
	}
	group := rec.GroupName
	if group == "" {
		group = "default"
	}
	return &model.APIKeyConfig{
		Key:           rec.Key,
		TenantID:      rec.TenantID,
		AllowedModels: rec.AllowedModels,
		RPM:           rec.RPM,
		TPM:           rec.TPM,
		Budget:        rec.Budget,
		UsedCost:      rec.UsedCost,
		BudgetUnits:   rec.BudgetUnits,
		UsedCostUnits: rec.UsedCostUnits,
		Status:        rec.Status,
		GroupName:     group,
		UserID:        rec.UserID,
		UserStatus:    rec.UserStatus,
		UserRole:      rec.UserRole,
		UserBalance:   rec.UserBalance,
	}
}

// mcpDataPlaneDenial renders a middleware admission failure as an MCP rejection.
func mcpDataPlaneDenial(denial *middleware.AccessDenial) *mcpAccessError {
	if denial == nil {
		return nil
	}
	return &mcpAccessError{
		StatusCode: denial.StatusCode,
		Code:       -32000,
		Message:    denial.Code + ": " + denial.Message,
	}
}

// respondMCPAccessError writes an access rejection as a JSON-RPC error.
func respondMCPAccessError(c *gin.Context, accessErr *mcpAccessError) {
	if accessErr == nil {
		return
	}
	c.JSON(accessErr.StatusCode, mcpResponse{
		JSONRPC: "2.0",
		Error:   &mcpError{Code: accessErr.Code, Message: accessErr.Message},
	})
}

// checkCallerRateLimits enforces the caller key's TPM/RPM budget, which the
// /mcp/* routes never got from RateLimitMiddleware. It returns false once the
// rejection has been written.
func (h *MCPHandler) checkCallerRateLimits(ctx context.Context, c *gin.Context, caller *mcpCaller) bool {
	if caller == nil || caller.Config == nil {
		return true
	}
	outcome := middleware.CheckKeyRateLimits(ctx, caller.Config)
	if outcome.Denial == nil {
		return true
	}
	for name, value := range outcome.Denial.Headers {
		c.Header(name, value)
	}
	respondMCPAccessError(c, mcpDataPlaneDenial(outcome.Denial))
	return false
}

// callerFromContext returns the caller bound by the transport, if any.
func callerFromContext(ctx context.Context) *mcpCaller {
	if ctx == nil {
		return nil
	}
	caller, _ := ctx.Value(contextKeyCallerRecord).(*mcpCaller)
	return caller
}

// mcpUsageIdentity is the caller attribution recorded for MCP usage. HTTP
// handlers derive the same values from the gin context; MCP has none.
type mcpUsageIdentity struct {
	apiKey    string
	tenantID  string
	keyGroup  string
	traceID   string
	keyConfig *model.APIKeyConfig
}

// newMCPUsageIdentity builds the attribution for one MCP tool call.
func newMCPUsageIdentity(caller *mcpCaller) mcpUsageIdentity {
	identity := mcpUsageIdentity{
		apiKey:   "mcp-session",
		tenantID: "mcp",
		keyGroup: "default",
		traceID:  middleware.GenerateTraceID(),
	}
	if caller == nil || caller.Record == nil {
		return identity
	}
	if caller.Record.Key != "" {
		identity.apiKey = caller.Record.Key
	}
	if caller.Record.TenantID != "" {
		identity.tenantID = caller.Record.TenantID
	}
	if caller.Config != nil {
		identity.keyConfig = caller.Config
		if caller.Config.GroupName != "" {
			identity.keyGroup = caller.Config.GroupName
		}
	}
	return identity
}

// validateMCPModelCurrency rejects a legacy foreign-currency rate before any
// upstream work starts, matching the HTTP data-plane pre-flight check. The
// pricing snapshot recorded afterwards lets settlement validate it again.
func validateMCPModelCurrency(modelName, keyGroup string) error {
	if billing.GlobalEngine == nil {
		return nil
	}
	if keyGroup == "" {
		keyGroup = "default"
	}
	return storage.ValidatePriceCurrency(billing.GlobalEngine.GetPriceWithGroup(modelName, keyGroup))
}
