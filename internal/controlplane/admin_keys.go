package controlplane

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListAPIKeys returns all API keys (scoped to current user if non-admin).
func (h *AdminHandler) ListAPIKeys(c *gin.Context) {
	if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
		user, err := h.repo.GetUserByUsername(claims.Username)
		if err != nil || user == nil {
			// Never fall through to the global key list: an unresolvable account
			// must fail closed instead of exposing other tenants' credentials.
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "账户不存在或暂时无法验证，请重新登录"})
			return
		}
		keys, err := h.repo.ListAPIKeysByUser(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
		return
	}

	keys, err := h.repo.ListAPIKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
}

// CreateAPIKeyRequest is the wire shape of an API key creation.
//
// It is deliberately separate from storage.APIKeyRecord. Binding the row
// straight onto the request body made every storage change an API change, and it
// let a client post id, used_cost, used_tokens, budget_units, used_cost_units and
// the timestamps: columns this path never reads from the body, but which the API
// appeared to accept.
//
// The accepted set is exactly the columns CreateAPIKey writes.
// TestCreateAPIKeyRequestAcceptsExactlyTheWritableColumns enforces that, so a new
// column cannot silently leave the API behind.
//
// Note the asymmetry with CreateUserKeyRequest in user_keys_handler.go: the
// user-facing endpoint already had its own request type. This closes that gap.
type CreateAPIKeyRequest struct {
	Key              string   `json:"key"`
	TenantID         string   `json:"tenant_id"`
	UserID           int64    `json:"user_id"`
	AllowedModels    []string `json:"allowed_models"`
	RPM              int      `json:"rpm"`
	TPM              int      `json:"tpm"`
	Budget           float64  `json:"budget"`
	GroupName        string   `json:"group_name"`
	Status           string   `json:"status"`
	FormatValidation string   `json:"format_validation"`
}

// toRecord converts the request into the row the repository persists. Usage
// counters and the integer ledger mirrors are not part of the request: the
// repository recomputes the mirrors from the float columns on insert.
func (r CreateAPIKeyRequest) toRecord() *storage.APIKeyRecord {
	return &storage.APIKeyRecord{
		Key:              r.Key,
		TenantID:         r.TenantID,
		UserID:           r.UserID,
		AllowedModels:    r.AllowedModels,
		RPM:              r.RPM,
		TPM:              r.TPM,
		Budget:           r.Budget,
		GroupName:        r.GroupName,
		Status:           r.Status,
		FormatValidation: r.FormatValidation,
	}
}

// CreateAPIKey generates a new API key.
func (h *AdminHandler) CreateAPIKey(c *gin.Context) {
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claims, ok := GetClaimsFromContext(c)
	if !ok {
		claims = nil
	}

	outcome := h.keyCreation.PlanCreate(claims, req)
	if !outcome.Allowed {
		writeKeyCreationDenial(c, outcome)
		return
	}

	if err := h.repo.CreateAPIKey(outcome.Record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.syncDataPlane("api_key_created")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": outcome.Record, "message": "API 密钥已成功创建并同步"})
}

// writeKeyCreationDenial renders a key creation refusal. Every branch keeps the
// exact status and payload the endpoint returned before the rules were extracted.
func writeKeyCreationDenial(c *gin.Context, outcome KeyCreationOutcome) {
	switch outcome.Denial {
	case KeyCreationAccountUnknown:
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账户不存在或暂时无法验证，请重新登录"})
	case KeyCreationGroupForbidden:
		c.JSON(http.StatusForbidden, gin.H{"error": outcome.Message})
	case KeyCreationEntropyUnavailable:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安全随机源不可用，无法生成密钥"})
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "无法创建密钥"})
	}
}

// currentKeyClaims reads the caller's claims, treating an absent context as no
// session so the rules can stay free of gin.
func currentKeyClaims(c *gin.Context) *AdminClaims {
	claims, ok := GetClaimsFromContext(c)
	if !ok {
		return nil
	}
	return claims
}

// checkKeyOwnership verifies whether the caller has rights to access or mutate the given API key.
//
// The decision lives in KeyAccessRules; this only renders it, so the rule can be
// exercised without a router. The signature is unchanged so the two callers do
// not have to care.
func (h *AdminHandler) checkKeyOwnership(c *gin.Context, keyRecord *storage.APIKeyRecord, forbiddenMsg string) (*storage.UserRecord, bool, bool) {
	access := h.keyAccess.CheckOwnership(currentKeyClaims(c), keyRecord, forbiddenMsg)
	if !access.Allowed {
		writeKeyAccessDenial(c, access)
		return nil, false, false
	}
	return access.User, access.IsAdmin, true
}

// writeKeyUpdateDenial renders an update refusal. The group branch keeps the exact
// status and message the handler produced before the rule was extracted.
func writeKeyUpdateDenial(c *gin.Context, outcome KeyUpdateOutcome) {
	switch outcome.Denial {
	case KeyUpdateGroupForbidden:
		c.JSON(http.StatusForbidden, gin.H{"error": outcome.Message})
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "无权修改该密钥"})
	}
}

// writeKeyAccessDenial renders an ownership refusal. Every branch keeps the exact
// status and payload the endpoints returned before the rule was extracted.
func writeKeyAccessDenial(c *gin.Context, access KeyAccess) {
	switch access.Denial {
	case KeyAccessNoSession:
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "请先登录账号"})
	case KeyAccessUserUnknown:
		c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
	case KeyAccessLookupFailed:
		c.JSON(http.StatusInternalServerError, gin.H{"error": access.Message})
	default:
		msg := access.Message
		if msg == "" {
			msg = "无权操作该密钥"
		}
		c.JSON(http.StatusForbidden, gin.H{"error": msg})
	}
}

// UpdateAPIKey updates an existing API key (e.g. status toggle, RPM, tenant, allowed models, group_name).
func (h *AdminHandler) UpdateAPIKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的密钥 ID"})
		return
	}

	existing, err := h.repo.GetAPIKey(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定 API 密钥"})
		return
	}

	access := h.keyAccess.CheckOwnership(currentKeyClaims(c), existing, "无权修改该密钥")
	if !access.Allowed {
		writeKeyAccessDenial(c, access)
		return
	}

	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	outcome := ApplyKeyUpdate(access, existing, req)
	if !outcome.Allowed {
		writeKeyUpdateDenial(c, outcome)
		return
	}

	if err := h.repo.UpdateAPIKey(outcome.Record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.syncDataPlane("api_key_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": outcome.Record, "message": "API 密钥已成功更新"})
}

// DeleteAPIKey removes an API key.
func (h *AdminHandler) DeleteAPIKey(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的密钥 ID"})
		return
	}

	existing, err := h.repo.GetAPIKey(id)
	if err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定 API 密钥"})
		return
	}

	if _, _, ok := h.checkKeyOwnership(c, existing, "无权操作该密钥"); !ok {
		return
	}

	if err := h.repo.DeleteAPIKey(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.syncDataPlane("api_key_deleted")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "API 密钥已成功删除"})
}

// isWellFormedAPIKey reports whether a client-supplied key looks like a real
// secret (URL-safe characters only). Weak/short values are regenerated.
func isWellFormedAPIKey(key string) bool {
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
