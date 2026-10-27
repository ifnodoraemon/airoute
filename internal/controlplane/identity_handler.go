package controlplane

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// RegisterRequest defines user registration payload.
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Code     string `json:"code"`
}

// Register registers a new regular user account with email verification and creates initial balance.
func (h *AdminHandler) Register(c *gin.Context) {
	if !config.GetGlobalConfig().IsRegistrationAllowed() {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "企业已关闭自主注册通道，请联系管理员分配账号"})
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名、邮箱与密码为必填项"})
		return
	}

	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)

	if len(username) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名至少需要 3 个字符"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "密码长度至少需要 6 个字符"})
		return
	}

	// Verify verification code if required
	if isEmailVerificationRequired() {
		if code == "" || !h.repo.VerifyCode(email, code, "register") {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "验证码无效或已过期，请重新获取"})
			return
		}
	} else if code != "" {
		_ = h.repo.VerifyCode(email, code, "register")
	}

	// Check if username or email already exists
	existingUser, _ := h.repo.GetUserByUsername(username)
	if existingUser != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该用户名已被注册"})
		return
	}
	existingEmail, _ := h.repo.GetUserByEmail(email)
	if existingEmail != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该邮箱已被绑定注册"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "密码加密失败"})
		return
	}

	trialBalance := config.GetGlobalConfig().GetInitialUserBalance()
	user := &storage.UserRecord{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		Balance:      trialBalance,
		GroupName:    "default",
	}

	// Automatically generate an initial API key for the new user
	newKey, err := GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "安全随机源不可用，无法生成密钥"})
		return
	}

	if err := h.repo.CreateUserWithAPIKey(user, &storage.APIKeyRecord{
		Key:           newKey,
		TenantID:      username,
		UserID:        user.ID,
		GroupName:     "default",
		AllowedModels: []string{"*"},
		RPM:           60,
		TPM:           100000,
		Budget:        100.0,
		Status:        "active",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建账号及初始密钥失败，请重试"})
		return
	}
	h.syncDataPlane()

	// Generate login token
	token, _ := GenerateAdminToken(user, 7*24*time.Hour)
	SetSessionCookies(c, token)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("注册成功！初始额度为 ¥%.2f，已自动生成 API 密钥", user.Balance),
		"data": gin.H{
			"token": token,
			"user": gin.H{
				"id":         user.ID,
				"username":   user.Username,
				"email":      user.Email,
				"role":       user.Role,
				"status":     user.Status,
				"balance":    user.Balance,
				"group_name": user.GroupName,
			},
			"api_key": newKey,
		},
	})
}
