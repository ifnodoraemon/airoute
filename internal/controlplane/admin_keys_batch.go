package controlplane

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// filterAuthorizedKeyIDs scopes batch operations to keys owned by the current non-admin user.
func (h *AdminHandler) filterAuthorizedKeyIDs(c *gin.Context, requestedIDs []int64) ([]int64, bool) {
	claims, ok := GetClaimsFromContext(c)
	if !ok || claims.Role == "admin" {
		return requestedIDs, true
	}

	user, _ := h.repo.GetUserByUsername(claims.Username)
	if user == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
		return nil, false
	}
	userKeys, err := h.repo.ListAPIKeysByUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	allowedMap := make(map[int64]bool, len(userKeys))
	for _, k := range userKeys {
		allowedMap[k.ID] = true
	}
	var filteredIDs []int64
	for _, id := range requestedIDs {
		if allowedMap[id] {
			filteredIDs = append(filteredIDs, id)
		}
	}
	if len(filteredIDs) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作所选密钥"})
		return nil, false
	}
	return filteredIDs, true
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
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "keys_batch_deleted")
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
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "keys_batch_status_updated")
	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个 API 访问密钥", action, n)})
}
