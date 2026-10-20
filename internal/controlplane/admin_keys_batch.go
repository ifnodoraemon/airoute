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

	n, err := h.repo.BatchUpdateAPIKeyStatus(targetIDs, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.syncDataPlane("keys_batch_status_updated")
	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个 API 访问密钥", action, n)})
}
