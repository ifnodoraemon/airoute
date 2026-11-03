package controlplane

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListUserKeys returns API keys owned by current user.
func (h *AdminHandler) ListUserKeys(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	keys, err := h.repo.ListAPIKeysByUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "查询密钥列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": apiKeyViews(keys)})
}
