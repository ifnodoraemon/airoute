package controlplane

import (
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// CreateUser provisions an account and its initial API key. Extracted from
// user_admin_handler.go to keep the user administration file under the size red line.
func (h *AdminHandler) CreateUser(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可创建账号")
	if !ok {
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名和密码不能为空"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名至少需要 3 个字符"})
		return
	}
	if isReservedUsername(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该用户名前缀为第三方登录保留，请更换用户名"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "密码长度至少需要 6 个字符"})
		return
	}

	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.GroupName = strings.ToLower(strings.TrimSpace(req.GroupName))
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if (req.Role != "user" && req.Role != "admin") || (req.Status != "active" && req.Status != "locked") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "角色仅支持 admin/user，状态仅支持 active/locked"})
		return
	}
	if req.GroupName == "" {
		req.GroupName = "default"
	}
	initialBalance := config.GetGlobalConfig().GetInitialUserBalance()
	if req.Balance != nil {
		initialBalance = *req.Balance
	} else if req.Role == "admin" {
		initialBalance = 9999999.0
	}
	if math.IsNaN(initialBalance) || math.IsInf(initialBalance, 0) || initialBalance < 0 || initialBalance > 1_000_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "初始余额必须在 0 至 1000000000 元之间"})
		return
	}

	// Account identity is case-insensitive (migration 44): "Alice" and "alice" are
	// the same name, so the duplicate check must not be byte-exact.
	existing, _ := h.repo.GetUserByUsername(req.Username)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该用户名已存在（用户名不区分大小写）"})
		return
	}
	if req.Email != "" {
		if existing, _ := h.repo.GetUserByEmail(req.Email); existing != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该邮箱已被绑定注册"})
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "密码加密失败"})
		return
	}

	user := &storage.UserRecord{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         req.Role,
		Status:       req.Status,
		Balance:      initialBalance,
		GroupName:    req.GroupName,
	}
	// Auto-generate initial API key for the new user
	newKey, err := GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安全随机源不可用，无法生成密钥"})
		return
	}
	if err := h.repo.CreateUserWithAPIKey(user, &storage.APIKeyRecord{
		Key:           newKey,
		TenantID:      user.Username,
		UserID:        user.ID,
		GroupName:     user.GroupName,
		AllowedModels: []string{"*"},
		RPM:           60,
		TPM:           100000,
		Budget:        100.0,
		Status:        "active",
	}); err != nil {
		// A concurrent create can lose the uniqueness race; report it as the client
		// error it is instead of a generic 500.
		if errors.Is(err, storage.ErrUsernameAlreadyRegistered) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "error": "该用户名已被注册（用户名不区分大小写）"})
			return
		}
		if errors.Is(err, storage.ErrEmailAlreadyRegistered) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "error": "该邮箱已被绑定注册"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建账号及初始密钥失败，请重试"})
		return
	}
	h.syncDataPlane()

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "创建账号成功",
		"data": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"balance":    user.Balance,
			"group_name": user.GroupName,
			"api_key":    newKey,
		},
	})
}