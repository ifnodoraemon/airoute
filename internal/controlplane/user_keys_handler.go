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

// CreateUserKeyRequest defines personal API key creation.
//
// TenantID is accepted for wire compatibility with older consoles but is ignored:
// the tenant of a self-service key is the authenticated account (see PlanUserKey),
// so a caller cannot label its traffic as another tenant. Name stays as the key's
// display name only.
type CreateUserKeyRequest struct {
	Name          string   `json:"name"`
	TenantID      string   `json:"tenant_id"`
	AllowedModels []string `json:"allowed_models"`
	RPM           int      `json:"rpm"`
	TPM           int      `json:"tpm"`
	Budget        float64  `json:"budget"`
}

// CreateUserKey generates a new API key for the current user.
func (h *AdminHandler) CreateUserKey(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	var req CreateUserKeyRequest
	_ = c.ShouldBindJSON(&req)

	newKey, err := GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安全随机源不可用，无法生成密钥"})
		return
	}

	rec := PlanUserKey(user, req, newKey)

	if err := h.repo.CreateAPIKey(rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建 API 密钥失败: " + err.Error()})
		return
	}
	h.syncDataPlane()

	// CreateAPIKey stores only the digest and leaves it in rec.Key, so the
	// plaintext (held in newKey) is returned exactly once, in this response.
	view := apiKeyViewFrom(rec)
	view.Key = newKey
	c.JSON(http.StatusOK, gin.H{
		"code":              0,
		"key_returned_once": true,
		"message":           "API 密钥创建成功",
		"data":              &view,
	})
}
