package controlplane

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"golang.org/x/crypto/bcrypt"
)

// LoginRequest defines credentials.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handles user authentication.
func (h *AdminHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名和密码不能为空"})
		return
	}

	user, err := h.repo.GetUserByUsername(req.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "用户名或密码错误"})
		return
	}

	if strings.EqualFold(user.Status, "locked") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "该账户已被管理员锁定，无法登录"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "用户名或密码错误"})
		return
	}

	expiryHours := config.GetGlobalConfig().GetTokenExpiryHours()
	token, err := GenerateAdminToken(user.Username, user.Role, time.Duration(expiryHours)*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "生成鉴权 Token 失败"})
		return
	}

	isDefaultPass := false
	if strings.EqualFold(user.Role, "admin") {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("admin123")); err == nil {
			isDefaultPass = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"token": token,
			"user": gin.H{
				"id":                  user.ID,
				"username":            user.Username,
				"email":               user.Email,
				"role":                user.Role,
				"status":              user.Status,
				"balance":             user.Balance,
				"is_admin":            strings.EqualFold(user.Role, "admin"),
				"group_name":          user.GroupName,
				"is_default_password": isDefaultPass,
			},
		},
		"message": "登录成功",
	})
}

// GetMe returns current authenticated user information.
func (h *AdminHandler) GetMe(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"username":            claims.Username,
				"role":                claims.Role,
				"status":              "active",
				"balance":             0.0,
				"is_admin":            strings.EqualFold(claims.Role, "admin"),
				"group_name":          "default",
				"is_default_password": false,
			},
		})
		return
	}

	isDefaultPass := false
	if strings.EqualFold(user.Role, "admin") {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("admin123")); err == nil {
			isDefaultPass = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"id":                  user.ID,
			"username":            user.Username,
			"email":               user.Email,
			"role":                user.Role,
			"status":              user.Status,
			"balance":             user.Balance,
			"is_admin":            strings.EqualFold(user.Role, "admin"),
			"group_name":          user.GroupName,
			"is_default_password": isDefaultPass,
		},
	})
}

// ChangePasswordRequest defines password update.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword allows admin or user to change their password.
func (h *AdminHandler) ChangePassword(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请求参数不完整"})
		return
	}

	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "新密码长度至少需要 6 个字符"})
		return
	}

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "旧密码验证失败"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "密码加密失败"})
		return
	}

	if err := h.repo.UpdateUserPassword(user.Username, string(newHash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新密码失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "密码修改成功，请使用新密码重新登录"})
}
