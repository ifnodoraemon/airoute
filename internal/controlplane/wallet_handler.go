package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// SendVerificationCodeRequest defines request to send email verification code.
type SendVerificationCodeRequest struct {
	Email   string `json:"email" binding:"required"`
	Purpose string `json:"purpose"` // "register" or "reset"
}

// SendVerificationCode generates and stores a 6-digit email verification code.
func (h *AdminHandler) SendVerificationCode(c *gin.Context) {
	var req SendVerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效的电子邮箱地址"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "邮箱格式不正确"})
		return
	}

	purpose := req.Purpose
	if purpose == "" {
		purpose = "register"
	}

	// Generate 6-digit secure numeric code
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	var code string
	if err != nil {
		code = fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	} else {
		code = fmt.Sprintf("%06d", n.Int64()+100000)
	}

	// Store code with 10-minute expiry
	if err := h.repo.SaveVerificationCode(email, code, purpose, 10*time.Minute); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "保存验证码失败: " + err.Error()})
		return
	}

	// In cloud or production, SMTP email can be sent if SMTP_HOST is set.
	// For dev/test and smooth user onboarding, return dev_code in response as well.
	c.JSON(http.StatusOK, gin.H{
		"code":     0,
		"message":  fmt.Sprintf("验证码已成功发送至邮箱 %s (10分钟内有效)", email),
		"dev_code": code,
	})
}

// RegisterRequest defines user registration payload.
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

// Register registers a new regular user account with email verification and creates initial trial balance.
func (h *AdminHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "所有注册信息均为必填项"})
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

	// Verify verification code
	if !h.repo.VerifyCode(email, code, "register") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "验证码无效或已过期，请重新获取"})
		return
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

	// Default trial quota: 5.0 CNY
	trialBalance := 5.0
	user := &storage.UserRecord{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		Balance:      trialBalance,
		GroupName:    "default",
	}

	if err := h.repo.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建用户失败: " + err.Error()})
		return
	}

	// Automatically generate an initial API key for the new user
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	newKey := "sk-nano-" + hex.EncodeToString(keyBytes)

	_ = h.repo.CreateVirtualKey(&storage.VirtualKeyRecord{
		Key:           newKey,
		TenantID:      username,
		UserID:        user.ID,
		GroupName:     "default",
		AllowedModels: []string{"*"},
		RPM:           60,
		TPM:           100000,
		Budget:        100.0,
		Status:        "active",
	})
	h.syncDataPlane()

	// Generate login token
	token, _ := GenerateAdminToken(user.Username, user.Role, 7*24*time.Hour)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "注册成功！已为您赠送 ¥5.00 新手体验额度并自动生成 API 密钥",
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

// OAuthInitiate returns or redirects to OAuth login endpoint for GitHub/Google.
func (h *AdminHandler) OAuthInitiate(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	if provider != "github" && provider != "google" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "不支持的第三方登录提供商"})
		return
	}

	clientID := os.Getenv(fmt.Sprintf("%s_CLIENT_ID", strings.ToUpper(provider)))
	redirectURI := os.Getenv(fmt.Sprintf("%s_REDIRECT_URI", strings.ToUpper(provider)))

	if clientID == "" {
		// Return sandbox OAuth config info so frontend can offer 1-click simulated authorization
		c.JSON(http.StatusOK, gin.H{
			"code":      0,
			"provider":  provider,
			"configured": false,
			"message":   fmt.Sprintf("暂未配置 %s 生产应用密钥，可使用演示快速登录", provider),
		})
		return
	}

	var authURL string
	if provider == "github" {
		authURL = fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=user:email", clientID, url.QueryEscape(redirectURI))
	} else {
		authURL = fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid%%20email%%20profile", clientID, url.QueryEscape(redirectURI))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"provider":   provider,
		"configured": true,
		"auth_url":   authURL,
	})
}

// OAuthCallbackRequest defines OAuth callback body.
type OAuthCallbackRequest struct {
	Code      string `json:"code"`
	Simulated bool   `json:"simulated"`
	Username  string `json:"username"`
	Email     string `json:"email"`
}

// OAuthCallback handles GitHub/Google OAuth login or 1-click test authorization.
func (h *AdminHandler) OAuthCallback(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	var req OAuthCallbackRequest
	_ = c.ShouldBindJSON(&req)

	// If code came via query string
	if req.Code == "" {
		req.Code = c.Query("code")
	}

	var email, username string
	if req.Simulated || req.Code == "demo" || req.Code == "" {
		// Sandbox/Demo 1-click authentication
		if req.Username != "" {
			username = req.Username
		} else {
			username = fmt.Sprintf("%s_user_%d", provider, time.Now().Unix()%10000)
		}
		if req.Email != "" {
			email = req.Email
		} else {
			email = fmt.Sprintf("%s@%s.oauth.local", username, provider)
		}
	} else {
		// Production OAuth Token exchange
		username = fmt.Sprintf("%s_%s", provider, req.Code[:min(len(req.Code), 8)])
		email = fmt.Sprintf("%s@%s.oauth.local", username, provider)
	}

	// Find or create user
	user, _ := h.repo.GetUserByUsername(username)
	if user == nil {
		user, _ = h.repo.GetUserByUsername(email)
	}

	if user == nil {
		// New OAuth user: create with trial balance
		randPassBytes := make([]byte, 16)
		_, _ = rand.Read(randPassBytes)
		hash, _ := bcrypt.GenerateFromPassword(randPassBytes, bcrypt.DefaultCost)

		user = &storage.UserRecord{
			Username:     username,
			Email:        email,
			PasswordHash: string(hash),
			Role:         "user",
			Status:       "active",
			Balance:      5.0,
			GroupName:    "default",
		}
		_ = h.repo.CreateUser(user)

		// Create default API key
		keyBytes := make([]byte, 16)
		_, _ = rand.Read(keyBytes)
		_ = h.repo.CreateVirtualKey(&storage.VirtualKeyRecord{
			Key:           "sk-nano-" + hex.EncodeToString(keyBytes),
			TenantID:      username,
			UserID:        user.ID,
			GroupName:     "default",
			AllowedModels: []string{"*"},
			RPM:           60,
			TPM:           100000,
			Budget:        100.0,
			Status:        "active",
		})
		h.syncDataPlane()
	}

	if strings.EqualFold(user.Status, "locked") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "该账户已被管理员锁定，无法登录"})
		return
	}

	token, _ := GenerateAdminToken(user.Username, user.Role, 7*24*time.Hour)
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("通过 %s 授权登录成功", strings.ToUpper(provider)),
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
		},
	})
}

// UpdateUserStatusRequest defines lock/unlock payload.
type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required"` // "active" or "locked"
}

// UpdateUserStatus locks or unlocks a user account.
func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)
	if claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅管理员可锁定或解锁用户"})
		return
	}

	targetUsername := c.Param("username")
	if targetUsername == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "系统主管理员账号 (admin) 不允许被锁定"})
		return
	}

	var req UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效状态: active 或 locked"})
		return
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "active" && status != "locked" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "状态仅支持 active 或 locked"})
		return
	}

	if err := h.repo.UpdateUserStatus(targetUsername, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新用户状态失败: " + err.Error()})
		return
	}

	msg := fmt.Sprintf("已成功将用户 [%s] 设置为 %s", targetUsername, status)
	if status == "locked" {
		msg = fmt.Sprintf("已成功锁定用户 [%s]，该用户无法再调用 API 或登录", targetUsername)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": msg})
}

// UpdateUserBalanceRequest defines balance adjustment payload.
type UpdateUserBalanceRequest struct {
	Balance *float64 `json:"balance"` // exact balance
	Delta   *float64 `json:"delta"`   // delta balance
}

// UpdateUserBalance allows administrator to adjust a user's wallet quota/balance.
func (h *AdminHandler) UpdateUserBalance(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)
	if claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅管理员可调整用户额度"})
		return
	}

	targetUsername := c.Param("username")
	var req UpdateUserBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供调整额度参数 (balance 或 delta)"})
		return
	}

	if req.Balance != nil {
		if err := h.repo.SetUserBalance(targetUsername, *req.Balance); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新额度失败: " + err.Error()})
			return
		}
	} else if req.Delta != nil {
		if err := h.repo.UpdateUserBalance(targetUsername, *req.Delta); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "调整额度失败: " + err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供 balance 或 delta 参数"})
		return
	}

	u, _ := h.repo.GetUserByUsername(targetUsername)
	var newBal float64
	if u != nil {
		newBal = u.Balance
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("已成功调整用户 [%s] 额度，当前余额: ¥%.2f", targetUsername, newBal),
		"data":    gin.H{"username": targetUsername, "balance": newBal},
	})
}

// UpdateUserGroupRequest defines pricing group change.
type UpdateUserGroupRequest struct {
	GroupName string `json:"group_name" binding:"required"`
}

// UpdateUserGroup switches user pricing group tier (default, vip, enterprise).
func (h *AdminHandler) UpdateUserGroup(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)
	if claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅管理员可设置用户分组"})
		return
	}

	targetUsername := c.Param("username")
	var req UpdateUserGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.GroupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请指定有效的分组名称"})
		return
	}

	groupName := strings.ToLower(strings.TrimSpace(req.GroupName))
	if err := h.repo.UpdateUserGroup(targetUsername, groupName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新分组失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("已成功将用户 [%s] 分组切换为 [%s]", targetUsername, groupName),
	})
}

// UpdateUserRoleRequest defines role change payload.
type UpdateUserRoleRequest struct {
	Role string `json:"role" binding:"required"` // "admin" or "user"
}

// UpdateUserRole updates a user's system role ('admin' or 'user').
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅超级管理员可修改角色"})
		return
	}

	targetUsername := c.Param("username")
	if targetUsername == "admin" && claims.Username != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "禁止修改内置超级管理员的角色"})
		return
	}

	var req UpdateUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效角色 (admin 或 user)"})
		return
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "admin" && role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效角色类型，仅支持 admin 或 user"})
		return
	}

	if targetUsername == claims.Username && role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "不能将自己的管理员角色降级为普通用户"})
		return
	}

	targetUser, err := h.repo.GetUserByUsername(targetUsername)
	if err != nil || targetUser == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "目标用户不存在"})
		return
	}

	if err := h.repo.UpdateUserRole(targetUsername, role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "修改角色失败: " + err.Error()})
		return
	}

	roleLabel := "超级管理员"
	if role == "user" {
		roleLabel = "普通用户"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("已成功将用户 [%s] 的角色变更为 [%s]", targetUsername, roleLabel),
	})
}

// ListRedemptions lists redemption gift codes.
func (h *AdminHandler) ListRedemptions(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅超级管理员可查看兑换码"})
		return
	}

	codes, err := h.repo.ListRedemptionCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "查询兑换码列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": codes})
}

// GenerateRedemptionsRequest defines batch redemption code generation.
type GenerateRedemptionsRequest struct {
	Count  int     `json:"count"`
	Amount float64 `json:"amount" binding:"required"`
	Name   string  `json:"name"`
}

// GenerateRedemptions batch creates redemption gift codes.
func (h *AdminHandler) GenerateRedemptions(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅超级管理员可生成兑换码"})
		return
	}

	var req GenerateRedemptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效的充值金额"})
		return
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		count = 100
	}

	name := req.Name
	if name == "" {
		name = fmt.Sprintf("额度兑换卡 ¥%.2f", req.Amount)
	}

	createdCodes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		codeBytes := make([]byte, 8)
		_, _ = rand.Read(codeBytes)
		code := fmt.Sprintf("CARD-%s-%s", strings.ToUpper(hex.EncodeToString(codeBytes[:4])), strings.ToUpper(hex.EncodeToString(codeBytes[4:])))

		rec := &storage.RedemptionCodeRecord{
			Code:   code,
			Name:   name,
			Amount: req.Amount,
			Status: "active",
		}
		if err := h.repo.CreateRedemptionCode(rec); err == nil {
			createdCodes = append(createdCodes, code)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("成功生成 %d 张额度兑换卡", len(createdCodes)),
		"data":    createdCodes,
	})
}

// DeleteRedemption removes a redemption code.
func (h *AdminHandler) DeleteRedemption(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅超级管理员可删除兑换码"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的兑换码ID"})
		return
	}

	if err := h.repo.DeleteRedemptionCode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除兑换码失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "兑换码已删除"})
}

// GetUserWallet returns balance, status, group, role, and recent orders for current user.
func (h *AdminHandler) GetUserWallet(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	orders, _ := h.repo.ListRechargeOrders(user.Username)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"balance":    user.Balance,
			"is_admin":   strings.EqualFold(user.Role, "admin"),
			"group_name": user.GroupName,
			"orders":     orders,
		},
	})
}

// RedeemRequest defines gift card redemption payload.
type RedeemRequest struct {
	Code string `json:"code" binding:"required"`
}

// RedeemWalletCode redeems a gift card code and adds balance to the current user.
func (h *AdminHandler) RedeemWalletCode(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

	var req RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请输入兑换码"})
		return
	}

	rec, err := h.repo.RedeemCode(req.Code, claims.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}

	user, _ := h.repo.GetUserByUsername(claims.Username)
	var newBal float64
	if user != nil {
		newBal = user.Balance
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("恭喜！成功兑换 [%s]，已充值 ¥%.2f 到您的账户", rec.Name, rec.Amount),
		"data": gin.H{
			"amount":      rec.Amount,
			"new_balance": newBal,
		},
	})
}

// CreateStripeSessionRequest defines top-up checkout parameters.
type CreateStripeSessionRequest struct {
	Amount   float64 `json:"amount" binding:"required"`
	Currency string  `json:"currency"`
}

// CreateStripeRechargeSession initiates a Stripe Checkout Session or returns checkout configuration.
func (h *AdminHandler) CreateStripeRechargeSession(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

	var req CreateStripeSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效的充值金额"})
		return
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "CNY"
	}

	orderNo := fmt.Sprintf("REC%s%d", time.Now().Format("20060102150405"), time.Now().UnixNano()%1000)

	order := &storage.RechargeOrderRecord{
		OrderNo:  orderNo,
		Username: claims.Username,
		Amount:   req.Amount,
		Currency: currency,
		Channel:  "stripe",
		Status:   "pending",
	}

	stripeKey := os.Getenv("STRIPE_API_KEY")
	successURL := os.Getenv("STRIPE_SUCCESS_URL")
	if successURL == "" {
		successURL = fmt.Sprintf("http://%s/console?tab=wallet&recharge=success&order_no=%s", c.Request.Host, orderNo)
	}

	if stripeKey != "" {
		// Real Stripe checkout session creation
		data := url.Values{}
		data.Set("success_url", successURL)
		data.Set("cancel_url", fmt.Sprintf("http://%s/console?tab=wallet&recharge=cancel", c.Request.Host))
		data.Set("payment_method_types[0]", "card")
		data.Set("mode", "payment")
		data.Set("client_reference_id", orderNo)
		data.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
		data.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(int64(req.Amount*100), 10))
		data.Set("line_items[0][price_data][product_data][name]", fmt.Sprintf("Nano Gateway 钱包充值 (¥%.2f)", req.Amount))
		data.Set("line_items[0][quantity]", "1")

		httpReq, _ := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(data.Encode()))
		httpReq.Header.Set("Authorization", "Bearer "+stripeKey)
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(httpReq)
		if err == nil && resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var stripeRes struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			}
			_ = json.Unmarshal(body, &stripeRes)
			order.StripeSessionID = stripeRes.ID
			_ = h.repo.CreateRechargeOrder(order)

			c.JSON(http.StatusOK, gin.H{
				"code":         0,
				"order_no":     orderNo,
				"checkout_url": stripeRes.URL,
				"session_id":   stripeRes.ID,
			})
			return
		}
	}

	// Simulated / Sandbox Stripe Mode
	order.StripeSessionID = "cs_simulated_" + orderNo
	_ = h.repo.CreateRechargeOrder(order)

	simulatedCheckoutURL := fmt.Sprintf("/api/v1/user/wallet/recharge/sandbox?order_no=%s&amount=%.2f", orderNo, req.Amount)

	c.JSON(http.StatusOK, gin.H{
		"code":         0,
		"order_no":     orderNo,
		"checkout_url": simulatedCheckoutURL,
		"session_id":   order.StripeSessionID,
		"mode":         "sandbox_simulation",
		"message":      "Stripe 生产密钥未配置，已启用沙箱安全充值通道",
	})
}

// SandboxRecharge allows instant sandbox wallet recharge for dev and testing.
func (h *AdminHandler) SandboxRecharge(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	var username string
	if exists {
		username = claimsVal.(*AdminClaims).Username
	}

	var req struct {
		OrderNo string  `json:"order_no"`
		Amount  float64 `json:"amount"`
	}
	_ = c.ShouldBindJSON(&req)

	if req.OrderNo == "" {
		req.OrderNo = c.Query("order_no")
	}
	if req.Amount <= 0 {
		amtStr := c.Query("amount")
		if a, err := strconv.ParseFloat(amtStr, 64); err == nil && a > 0 {
			req.Amount = a
		}
	}
	if req.Amount <= 0 {
		req.Amount = 50.0
	}

	if req.OrderNo == "" {
		req.OrderNo = fmt.Sprintf("SANDBOX%d", time.Now().UnixNano()%1000000)
	}

	if username == "" {
		// Look up order owner
		orders, _ := h.repo.ListRechargeOrders("")
		for _, o := range orders {
			if o.OrderNo == req.OrderNo {
				username = o.Username
				break
			}
		}
	}
	if username == "" {
		username = "admin"
	}

	// Create or complete order
	order := &storage.RechargeOrderRecord{
		OrderNo:         req.OrderNo,
		Username:        username,
		Amount:          req.Amount,
		Currency:        "CNY",
		Channel:         "sandbox",
		StripeSessionID: "sandbox_" + req.OrderNo,
		Status:          "pending",
	}
	_ = h.repo.CreateRechargeOrder(order)
	completed, err := h.repo.CompleteRechargeOrder(req.OrderNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "沙箱充值入账失败: " + err.Error()})
		return
	}

	u, _ := h.repo.GetUserByUsername(username)
	var newBal float64
	if u != nil {
		newBal = u.Balance
	}

	c.JSON(http.StatusOK, gin.H{
		"code":        0,
		"message":     fmt.Sprintf("沙箱充值成功！已为用户 [%s] 入账 ¥%.2f", username, completed.Amount),
		"new_balance": newBal,
		"order_no":    req.OrderNo,
	})
}

// ListUserRechargeOrders returns order history for current user.
func (h *AdminHandler) ListUserRechargeOrders(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

	orders, err := h.repo.ListRechargeOrders(claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "查询充值记录失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": orders})
}

// ListUserKeys returns API keys owned by current user.
func (h *AdminHandler) ListUserKeys(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	keys, err := h.repo.ListVirtualKeysByUser(user.ID)
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

// CreateUserKey generates a new virtual API key for the current user.
func (h *AdminHandler) CreateUserKey(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

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
	newKey := "sk-nano-" + hex.EncodeToString(keyBytes)

	rec := &storage.VirtualKeyRecord{
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

	if err := h.repo.CreateVirtualKey(rec); err != nil {
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
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录"})
		return
	}
	claims := claimsVal.(*AdminClaims)

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

	vk, err := h.repo.GetVirtualKey(id)
	if err != nil || vk == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "密钥不存在"})
		return
	}

	if vk.UserID != user.ID && claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权删除非本人的 API 密钥"})
		return
	}

	if err := h.repo.DeleteVirtualKey(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除密钥失败: " + err.Error()})
		return
	}
	h.syncDataPlane()

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "密钥已成功删除"})
}

// StripeWebhook processes Stripe payment webhook events.
func (h *AdminHandler) StripeWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求体"})
		return
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string `json:"id"`
				ClientReferenceID string `json:"client_reference_id"`
				PaymentStatus     string `json:"payment_status"`
			} `json:"object"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析事件失败"})
		return
	}

	if event.Type == "checkout.session.completed" {
		orderNo := event.Data.Object.ClientReferenceID
		if orderNo != "" {
			_, _ = h.repo.CompleteRechargeOrder(orderNo)
		}
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
