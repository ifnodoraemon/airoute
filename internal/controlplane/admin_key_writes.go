package controlplane

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// CreateAPIKey generates a new API key.
func (h *AdminHandler) CreateAPIKey(c *gin.Context) {
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Reject an unknown level instead of storing a value the data plane will never
	// match: an accepted-but-ignored field is what made this setting a no-op.
	formatValidation, err := normalizeFormatValidation(req.FormatValidation)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.FormatValidation = formatValidation

	claims, ok := GetClaimsFromContext(c)
	if !ok {
		claims = nil
	}

	outcome := h.keyCreation.PlanCreate(claims, req)
	if !outcome.Allowed {
		writeKeyCreationDenial(c, outcome)
		return
	}

	if !h.validateKeyPricingGroup(c, outcome.Record.GroupName) {
		return
	}

	// The repository stores only the digest and overwrites Record.Key with it, so
	// the plaintext has to be captured first: it is returned exactly once, here.
	plaintextKey := outcome.Record.Key
	if err := h.repo.CreateAPIKey(outcome.Record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.syncDataPlane("api_key_created")

	view := apiKeyViewFrom(outcome.Record)
	view.Key = plaintextKey
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": &view, "key_returned_once": true, "message": "API 密钥已成功创建并同步"})
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

	if req.GroupName != nil && !h.validateKeyPricingGroup(c, outcome.Record.GroupName) {
		return
	}

	if err := h.repo.UpdateAPIKey(outcome.Record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.syncDataPlane("api_key_updated")

	// Return the same projection create/list use. Handing out the storage row made
	// this endpoint's payload a function of storage's JSON tags rather than of the
	// API contract (see apiKeyView).
	view := apiKeyViewFrom(outcome.Record)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": &view, "message": "API 密钥已成功更新"})
}
