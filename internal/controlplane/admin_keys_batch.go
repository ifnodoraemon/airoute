package controlplane

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// filterAuthorizedKeyIDs scopes batch operations to keys owned by the current non-admin user.
//
// The scoping rule lives in KeyAccessRules; this only renders its decision.
func (h *AdminHandler) filterAuthorizedKeyIDs(c *gin.Context, requestedIDs []int64) ([]int64, bool) {
	ids, access := h.keyAccess.FilterAuthorizedIDs(currentKeyClaims(c), requestedIDs)
	if !access.Allowed {
		writeKeyAccessDenial(c, access)
		return nil, false
	}
	return ids, true
}

// BatchDeleteAPIKeys deletes multiple API keys.
func (h *AdminHandler) BatchDeleteAPIKeys(c *gin.Context) {
	ids, ok := h.bindBatchIDs(c, "请提供有效密钥 ID 列表")
	if !ok {
		return
	}

	targetIDs, ok := h.filterAuthorizedKeyIDs(c, ids)
	if !ok {
		return
	}

	n, err := h.repo.BatchDeleteAPIKeys(targetIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.syncDataPlane("keys_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量注销 %d 个 API 访问密钥", n), "deleted_count": n})
}

// BatchStatusAPIKeys toggles status for multiple API keys.
func (h *AdminHandler) BatchStatusAPIKeys(c *gin.Context) {
	req, ok := h.bindBatchStatus(c)
	if !ok {
		return
	}

	targetIDs, ok := h.filterAuthorizedKeyIDs(c, req.IDs)
	if !ok {
		return
	}

	// An administrator's bulk status change is a revocation decision and must be
	// durable; a tenant's is self-service and must not resurrect a key an operator
	// disabled (BUG-CP-01). The two are distinct repository operations.
	claims, _ := GetClaimsFromContext(c)
	callerIsAdmin := claims != nil && claims.Role == "admin"

	var n int64
	var err error
	if callerIsAdmin {
		n, err = h.repo.BatchUpdateAPIKeyStatusAsAdmin(targetIDs, req.Status)
	} else {
		n, err = h.repo.BatchUpdateAPIKeyStatus(targetIDs, req.Status)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}

	// A self-service "enable" that changed nothing means every selected key is
	// administratively revoked: say so instead of reporting a successful enable.
	if !callerIsAdmin && req.Status == "active" && n < int64(len(targetIDs)) {
		if n == 0 {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "所选密钥已被管理员停用，无法自行启用，请联系管理员"})
			return
		}
		h.syncDataPlane("keys_batch_status_updated")
		c.JSON(http.StatusOK, gin.H{
			"code":          0,
			"message":       fmt.Sprintf("已批量%s %d 个 API 访问密钥；%d 个已被管理员停用而跳过", action, n, int64(len(targetIDs))-n),
			"updated_count": n,
			"skipped_count": int64(len(targetIDs)) - n,
		})
		return
	}

	if n == 0 && len(targetIDs) > 0 {
		// Nothing matched (already in the requested state, or only administratively
		// revoked rows). Report it rather than a misleading "已批量启用 0 个".
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "所选密钥均已是目标状态或无变更", "updated_count": 0})
		return
	}

	h.syncDataPlane("keys_batch_status_updated")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个 API 访问密钥", action, n), "updated_count": n})
}
