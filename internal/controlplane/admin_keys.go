package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListAPIKeys returns all API keys (scoped to current user if non-admin).
func (h *AdminHandler) ListAPIKeys(c *gin.Context) {
	if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
		user, _ := h.repo.GetUserByUsername(claims.Username)
		if user != nil {
			keys, err := h.repo.ListAPIKeysByUser(user.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
			return
		}
	}

	keys, err := h.repo.ListAPIKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
}

// CreateAPIKey generates a new API key.
func (h *AdminHandler) CreateAPIKey(c *gin.Context) {
	var rec storage.APIKeyRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if claims, ok := GetClaimsFromContext(c); ok {
		if claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user != nil {
				rec.UserID = user.ID
				userGroup := strings.ToLower(strings.TrimSpace(user.GroupName))
				if userGroup == "" {
					userGroup = "default"
				}
				reqGroup := strings.ToLower(strings.TrimSpace(rec.GroupName))
				if reqGroup == "" {
					reqGroup = userGroup
				}
				if reqGroup != "default" && reqGroup != userGroup {
					c.JSON(http.StatusForbidden, gin.H{
						"error": fmt.Sprintf("无权使用分组 [%s]，您的账号保障分组等级为 [%s]", reqGroup, userGroup),
					})
					return
				}
				rec.GroupName = reqGroup
			}
		} else {
			if rec.UserID <= 0 {
				adminUser, _ := h.repo.GetUserByUsername(claims.Username)
				if adminUser != nil {
					rec.UserID = adminUser.ID
				}
			}
			if strings.TrimSpace(rec.GroupName) == "" {
				rec.GroupName = "default"
			} else {
				rec.GroupName = strings.ToLower(strings.TrimSpace(rec.GroupName))
			}
		}
	} else {
		if strings.TrimSpace(rec.GroupName) == "" {
			rec.GroupName = "default"
		} else {
			rec.GroupName = strings.ToLower(strings.TrimSpace(rec.GroupName))
		}
	}

	if rec.Key == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		rec.Key = fmt.Sprintf("sk-airoute-%s", hex.EncodeToString(b))
	}
	if rec.TenantID == "" {
		rec.TenantID = "default-app"
	}

	if err := h.repo.CreateAPIKey(&rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "api_key_created")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rec, "message": "API 密钥已成功创建并同步"})
}

// checkKeyOwnership verifies whether the caller has rights to access or mutate the given API key.
func (h *AdminHandler) checkKeyOwnership(c *gin.Context, keyRecord *storage.APIKeyRecord, forbiddenMsg string) (*storage.UserRecord, bool, bool) {
	claims, ok := GetClaimsFromContext(c)
	if !ok || claims.Role == "admin" {
		return nil, true, true
	}
	user, _ := h.repo.GetUserByUsername(claims.Username)
	if user == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
		return nil, false, false
	}
	if keyRecord == nil || keyRecord.UserID != user.ID {
		if forbiddenMsg == "" {
			forbiddenMsg = "无权操作该密钥"
		}
		c.JSON(http.StatusForbidden, gin.H{"error": forbiddenMsg})
		return nil, false, false
	}
	return user, false, true
}

// UpdateAPIKey updates an existing API key (e.g. status toggle, RPM, tenant, allowed models, group_name).
func (h *AdminHandler) UpdateAPIKey(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的密钥 ID"})
		return
	}

	existing, err := h.repo.GetAPIKey(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定 API 密钥"})
		return
	}

	currentUser, isAdmin, ok := h.checkKeyOwnership(c, existing, "无权修改该密钥")
	if !ok {
		return
	}

	var req struct {
		TenantID         *string   `json:"tenant_id"`
		AllowedModels    []string  `json:"allowed_models"`
		RPM              *int      `json:"rpm"`
		TPM              *int      `json:"tpm"`
		Budget           *float64  `json:"budget"`
		Status           *string   `json:"status"`
		FormatValidation *string   `json:"format_validation"`
		GroupName        *string   `json:"group_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TenantID != nil {
		existing.TenantID = *req.TenantID
	}
	if req.AllowedModels != nil {
		existing.AllowedModels = req.AllowedModels
	}
	if req.RPM != nil {
		existing.RPM = *req.RPM
	}
	if req.TPM != nil {
		existing.TPM = *req.TPM
	}
	if req.Budget != nil {
		existing.Budget = *req.Budget
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}
	if req.FormatValidation != nil {
		existing.FormatValidation = *req.FormatValidation
	}
	if req.GroupName != nil {
		targetGroup := strings.ToLower(strings.TrimSpace(*req.GroupName))
		if targetGroup == "" {
			targetGroup = "default"
		}
		if !isAdmin && currentUser != nil {
			userGroup := strings.ToLower(strings.TrimSpace(currentUser.GroupName))
			if userGroup == "" {
				userGroup = "default"
			}
			if targetGroup != "default" && targetGroup != userGroup {
				c.JSON(http.StatusForbidden, gin.H{
					"error": fmt.Sprintf("无权切换至分组 [%s]，您的账号保障分组等级为 [%s]", targetGroup, userGroup),
				})
				return
			}
		}
		existing.GroupName = targetGroup
	}

	if err := h.repo.UpdateAPIKey(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "api_key_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": existing, "message": "API 密钥已成功更新"})
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

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "api_key_deleted")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "API 密钥已成功删除"})
}
