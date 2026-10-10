package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// ListUsers returns all users with balance, status, role, and group.
func (h *AdminHandler) ListUsers(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可查看用户列表")
	if !ok {
		return
	}

	users, err := h.repo.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "查询用户列表失败: " + err.Error()})
		return
	}
	type UserDTO struct {
		ID        int64   `json:"id"`
		Username  string  `json:"username"`
		Email     string  `json:"email"`
		Role      string  `json:"role"`
		Status    string  `json:"status"`
		Balance   float64 `json:"balance"`
		GroupName string  `json:"group_name"`
		CreatedAt string  `json:"created_at"`
		UpdatedAt string  `json:"updated_at"`
	}
	res := make([]UserDTO, 0)
	for _, u := range users {
		res = append(res, UserDTO{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			Status:    u.Status,
			Balance:   u.Balance,
			GroupName: u.GroupName,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: u.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": res})
}

// CreateUserRequest defines payload to create a new user account.
type CreateUserRequest struct {
	Username  string  `json:"username" binding:"required"`
	Email     string  `json:"email"`
	Password  string  `json:"password" binding:"required"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
	Balance   float64 `json:"balance"`
	GroupName string  `json:"group_name"`
}

// CreateUser adds a new user with quota, group, and automatically creates an initial API key.
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
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "密码长度至少需要 6 个字符"})
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.GroupName == "" {
		req.GroupName = "default"
	}
	if req.Role == "admin" && req.Balance <= 0 {
		req.Balance = 9999999.0
	} else if req.Balance <= 0 {
		req.Balance = 10.0 // Default initial quota
	}

	existing, _ := h.repo.GetUserByUsername(req.Username)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该用户名已存在"})
		return
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
		Balance:      req.Balance,
		GroupName:    req.GroupName,
	}
	if err := h.repo.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建用户失败: " + err.Error()})
		return
	}

	// Auto-generate initial API key for the new user
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	newKey := "sk-airoute-" + hex.EncodeToString(keyBytes)
	_ = h.repo.CreateAPIKey(&storage.APIKeyRecord{
		Key:           newKey,
		TenantID:      user.Username,
		UserID:        user.ID,
		GroupName:     user.GroupName,
		AllowedModels: []string{"*"},
		RPM:           60,
		TPM:           100000,
		Budget:        100.0,
		Status:        "active",
	})
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

// DeleteUser deletes an account by username.
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	claims, ok := GetClaimsFromContext(c)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅超级管理员可删除账号"})
		return
	}

	targetUsername := c.Param("username")
	if targetUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请指定要删除的用户名"})
		return
	}

	if targetUsername == claims.Username {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无法删除当前正在登录的账号"})
		return
	}

	adminRoot := config.GetGlobalConfig().GetAdminUsername()
	if targetUsername == "admin" || targetUsername == adminRoot {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "系统初始管理员账号不允许删除"})
		return
	}

	if err := h.repo.DeleteUser(targetUsername); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除用户失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "账号已成功删除"})
}

// ResetUserPasswordRequest defines reset payload.
type ResetUserPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// ResetUserPassword allows administrator to reset password of any user.
func (h *AdminHandler) ResetUserPassword(c *gin.Context) {
	claims, ok := GetClaimsFromContext(c)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅管理员可重置密码"})
		return
	}

	targetUsername := c.Param("username")
	if targetUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请指定要重置密码的用户名"})
		return
	}

	var req ResetUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "新密码长度至少需要 6 个字符"})
		return
	}

	targetUser, err := h.repo.GetUserByUsername(targetUsername)
	if err != nil || targetUser == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "目标用户不存在"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "密码加密失败"})
		return
	}

	if err := h.repo.UpdateUserPassword(targetUsername, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "重置密码失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已成功重置用户 [%s] 的密码", targetUsername)})
}
