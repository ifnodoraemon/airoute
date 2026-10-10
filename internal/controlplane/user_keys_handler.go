package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
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
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
}

// CreateUserKeyRequest defines personal API key creation.
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

	if len(req.AllowedModels) == 0 {
		req.AllowedModels = []string{"*"}
	}
	rpm := req.RPM
	if rpm < 0 {
		rpm = 0
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(req.Name)
	}
	if tenantID == "" {
		tenantID = user.Username
	}
	budget := req.Budget
	if budget < 0 {
		budget = 0
	}
	tpm := req.TPM
	if tpm <= 0 {
		tpm = 100000
	}

	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	newKey := "sk-airoute-" + hex.EncodeToString(keyBytes)

	rec := &storage.APIKeyRecord{
		Key:           newKey,
		TenantID:      tenantID,
		UserID:        user.ID,
		GroupName:     user.GroupName,
		AllowedModels: req.AllowedModels,
		RPM:           rpm,
		TPM:           tpm,
		Budget:        budget,
		Status:        "active",
	}

	if err := h.repo.CreateAPIKey(rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建 API 密钥失败: " + err.Error()})
		return
	}
	h.syncDataPlane()

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "API 密钥创建成功",
		"data":    rec,
	})
}

// DeleteUserKey deletes an API key owned by the current user.
func (h *AdminHandler) DeleteUserKey(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的密钥ID"})
		return
	}

	k, err := h.repo.GetAPIKey(id)
	if err != nil || k == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "密钥不存在"})
		return
	}

	if k.UserID != user.ID && claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权删除非本人的 API 密钥"})
		return
	}

	if err := h.repo.DeleteAPIKey(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除密钥失败: " + err.Error()})
		return
	}
	h.syncDataPlane()

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "密钥已成功删除"})
}
