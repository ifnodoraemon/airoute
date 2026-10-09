package controlplane

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"golang.org/x/crypto/bcrypt"
)

// isEmailVerificationRequired checks if email verification is mandatory in this deployment.
func isEmailVerificationRequired() bool {
	return config.GetGlobalConfig().IsEmailVerificationRequired()
}

// sendVerificationEmail sends a 6-digit verification code via SMTP if configured.
func sendVerificationEmail(targetEmail, code, purpose string) error {
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if smtpHost == "" {
		telemetry.Logger.Info("email verification code generated (SMTP not configured, logged for audit)",
			"email", targetEmail, "purpose", purpose, "code", code)
		return nil
	}

	smtpPort := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if smtpPort == "" {
		smtpPort = "587"
	}
	smtpUser := strings.TrimSpace(os.Getenv("SMTP_USER"))
	smtpPass := os.Getenv("SMTP_PASS")
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if smtpFrom == "" {
		if smtpUser != "" {
			smtpFrom = smtpUser
		} else {
			smtpFrom = "noreply@" + smtpHost
		}
	}

	subject := "Airoute 验证码"
	actionName := "注册新账号"
	if purpose == "reset" {
		subject = "Airoute 密码重置验证码"
		actionName = "重置登录密码"
	}

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n"+
		"<!DOCTYPE html><html><body style=\"font-family:sans-serif;line-height:1.6;color:#333;\">"+
		"<div style=\"max-width:540px;margin:20px auto;padding:24px;border:1px solid #e2e8f0;border-radius:16px;\">"+
		"<h2 style=\"color:#4f46e5;margin-top:0;\">Airoute 智能网关安全验证</h2>"+
		"<p>您正在进行 <strong>%s</strong> 操作，本次安全验证码为：</p>"+
		"<div style=\"font-size:32px;font-weight:bold;letter-spacing:6px;color:#1e293b;background:#f1f5f9;padding:16px;text-align:center;border-radius:12px;margin:20px 0;\">%s</div>"+
		"<p style=\"color:#64748b;font-size:13px;\">验证码在 10 分钟内有效。如非本人操作，请忽略此邮件。</p>"+
		"</div></body></html>", smtpFrom, targetEmail, subject, actionName, code)

	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)
	var auth smtp.Auth
	if smtpUser != "" && smtpPass != "" {
		auth = smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	}

	return smtp.SendMail(addr, auth, smtpFrom, []string{targetEmail}, []byte(body))
}

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

	// Dispatch email via SMTP if configured
	if err := sendVerificationEmail(email, code, purpose); err != nil {
		telemetry.Logger.Error("failed to dispatch verification email via SMTP", "email", email, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "发送邮件失败: " + err.Error()})
		return
	}

	// In automated test or development mode, return dev_code for testing convenience
	isDevOrTest := strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/") || os.Getenv("ENV") == "development"
	resp := gin.H{
		"code":    0,
		"message": fmt.Sprintf("验证码已成功发送至邮箱 %s (10分钟内有效)", email),
	}
	if isDevOrTest {
		resp["dev_code"] = code
	}
	c.JSON(http.StatusOK, resp)
}

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

	if err := h.repo.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建用户失败: " + err.Error()})
		return
	}

	// Automatically generate an initial API key for the new user
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	newKey := "sk-airoute-" + hex.EncodeToString(keyBytes)

	_ = h.repo.CreateAPIKey(&storage.APIKeyRecord{
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
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/auth/oauth/%s/callback", ResolvePublicBaseURL(c), provider)
	}

	if clientID == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"code":       404,
			"provider":   provider,
			"configured": false,
			"error":      fmt.Sprintf("系统尚未配置 %s 生产应用密钥 (缺少 %s_CLIENT_ID 环境变量)", strings.ToUpper(provider), strings.ToUpper(provider)),
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

func exchangeGitHubOAuth(code, clientID, clientSecret string) (string, string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", err
	}
	if tokenResp.Error != "" {
		return "", "", fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", "", fmt.Errorf("empty access token returned")
	}

	userReq, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("User-Agent", "Airoute-Gateway")

	userResp, err := client.Do(userReq)
	if err != nil {
		return "", "", err
	}
	defer userResp.Body.Close()

	var ghUser struct {
		Login string `json:"login"`
		Email string `json:"email"`
		ID    int64  `json:"id"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil {
		return "", "", err
	}

	username := ghUser.Login
	if username == "" {
		username = fmt.Sprintf("gh_%d", ghUser.ID)
	}
	email := ghUser.Email
	if email == "" {
		email = fmt.Sprintf("%s@github.oauth.local", username)
	}

	return username, email, nil
}

func exchangeGoogleOAuth(code, clientID, clientSecret, redirectURI string) (string, string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", err
	}
	if tokenResp.AccessToken == "" {
		return "", "", fmt.Errorf("empty access token returned: %s", tokenResp.ErrorDesc)
	}

	userReq, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userResp, err := client.Do(userReq)
	if err != nil {
		return "", "", err
	}
	defer userResp.Body.Close()

	var ggUser struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ggUser); err != nil {
		return "", "", err
	}

	username := ggUser.Name
	if username == "" {
		username = "google_user_" + ggUser.ID
	}
	return username, ggUser.Email, nil
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

	clientID := os.Getenv(fmt.Sprintf("%s_CLIENT_ID", strings.ToUpper(provider)))
	clientSecret := os.Getenv(fmt.Sprintf("%s_CLIENT_SECRET", strings.ToUpper(provider)))
	redirectURI := os.Getenv(fmt.Sprintf("%s_REDIRECT_URI", strings.ToUpper(provider)))
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/auth/oauth/%s/callback", ResolvePublicBaseURL(c), provider)
	}

	isTestOrDev := strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/") || strings.EqualFold(os.Getenv("ENABLE_SIMULATED_OAUTH"), "true")

	var email, username string

	if clientID != "" && clientSecret != "" && req.Code != "" && req.Code != "demo" && !req.Simulated {
		// Real production token exchange
		if provider == "github" {
			realUser, realEmail, err := exchangeGitHubOAuth(req.Code, clientID, clientSecret)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "GitHub 授权失败: " + err.Error()})
				return
			}
			username = "gh_" + realUser
			email = realEmail
		} else if provider == "google" {
			realUser, realEmail, err := exchangeGoogleOAuth(req.Code, clientID, clientSecret, redirectURI)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "Google 授权失败: " + err.Error()})
				return
			}
			username = "gg_" + realUser
			email = realEmail
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "不支持的 OAuth 提供商"})
			return
		}
	} else if isTestOrDev {
		// Test/dev simulated authentication
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
		c.JSON(http.StatusForbidden, gin.H{
			"code":  403,
			"error": fmt.Sprintf("系统未配置 %s 生产应用密钥，禁止使用模拟快捷登录", strings.ToUpper(provider)),
		})
		return
	}

	// Find or create user
	user, _ := h.repo.GetUserByUsername(username)
	if user == nil {
		user, _ = h.repo.GetUserByUsername(email)
	}

	// Security: disallow demo/simulated OAuth to log in as administrator
	if (req.Simulated || req.Code == "demo" || req.Code == "") && user != nil && strings.EqualFold(user.Role, "admin") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "管理员账号不允许通过模拟演示登录，请使用标准密码登录"})
		return
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
		_ = h.repo.CreateAPIKey(&storage.APIKeyRecord{
			Key:           "sk-airoute-" + hex.EncodeToString(keyBytes),
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
	rootAdmin := config.GetGlobalConfig().GetAdminUsername()
	if targetUsername == "admin" || targetUsername == rootAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "系统初始管理员账号不允许被锁定"})
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
	rootAdmin := config.GetGlobalConfig().GetAdminUsername()
	if (targetUsername == "admin" || targetUsername == rootAdmin) && claims.Username != targetUsername {
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

	baseURL := ResolvePublicBaseURL(c)
	stripeKey := os.Getenv("STRIPE_API_KEY")
	successURL := os.Getenv("STRIPE_SUCCESS_URL")
	if successURL == "" {
		successURL = fmt.Sprintf("%s/app/?tab=wallet&recharge=success&order_no=%s", baseURL, orderNo)
	}

	if stripeKey != "" {
		// Real Stripe checkout session creation
		data := url.Values{}
		data.Set("success_url", successURL)
		data.Set("cancel_url", fmt.Sprintf("%s/app/?tab=wallet&recharge=cancel", baseURL))
		data.Set("payment_method_types[0]", "card")
		data.Set("mode", "payment")
		data.Set("client_reference_id", orderNo)
		data.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
		data.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(int64(req.Amount*100), 10))
		data.Set("line_items[0][price_data][product_data][name]", fmt.Sprintf("Airoute 钱包充值 (¥%.2f)", req.Amount))
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

	isTestOrDev := gin.Mode() == gin.TestMode || strings.HasSuffix(os.Args[0], ".test") || strings.EqualFold(os.Getenv("ENABLE_SANDBOX_RECHARGE"), "true")

	if stripeKey == "" {
		if !strings.EqualFold(claims.Role, "admin") && !isTestOrDev {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":  503,
				"error": "系统尚未配置 Stripe 线上收款通道 (缺少 STRIPE_API_KEY)。请联系管理员使用卡密兑换或对公结算。",
			})
			return
		}
	}

	// Simulated / Sandbox Stripe Mode (accessible to admins or in dev/test mode)
	order.StripeSessionID = "cs_simulated_" + orderNo
	_ = h.repo.CreateRechargeOrder(order)

	simulatedCheckoutURL := fmt.Sprintf("/api/v1/user/wallet/recharge/sandbox?order_no=%s&amount=%.2f", orderNo, req.Amount)

	c.JSON(http.StatusOK, gin.H{
		"code":         0,
		"order_no":     orderNo,
		"checkout_url": simulatedCheckoutURL,
		"session_id":   order.StripeSessionID,
		"mode":         "sandbox_simulation",
		"message":      "已启用测试快捷充值通道",
	})
}

// SandboxRecharge allows instant wallet recharge for direct/fast checkout (Admin/Dev/Test only).
func (h *AdminHandler) SandboxRecharge(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录或登录凭证已失效"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims == nil || claims.Username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "无效的认证凭证"})
		return
	}

	isTestOrDev := gin.Mode() == gin.TestMode || strings.HasSuffix(os.Args[0], ".test") || strings.EqualFold(os.Getenv("ENABLE_SANDBOX_RECHARGE"), "true")
	if !strings.EqualFold(claims.Role, "admin") && !isTestOrDev {
		c.JSON(http.StatusForbidden, gin.H{
			"code":  403,
			"error": "生产安全模式下已禁用沙箱充值。请使用企业兑换码或正规结算通道。",
		})
		return
	}

	username := claims.Username

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
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "充值入账失败: " + err.Error()})
		return
	}

	u, _ := h.repo.GetUserByUsername(username)
	var newBal float64
	if u != nil {
		newBal = u.Balance
	}

	c.JSON(http.StatusOK, gin.H{
		"code":        0,
		"message":     fmt.Sprintf("充值成功！已为用户 [%s] 入账 ¥%.2f", username, completed.Amount),
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

// VerifyStripeWebhookSignature validates Stripe webhook HMAC-SHA256 signature against webhook secret.
func VerifyStripeWebhookSignature(payload []byte, sigHeader, secret string, tolerance time.Duration) error {
	if secret == "" {
		return fmt.Errorf("STRIPE_WEBHOOK_SECRET 未配置")
	}
	if sigHeader == "" {
		return fmt.Errorf("缺少 Stripe-Signature 请求头")
	}

	var timestampStr string
	var signatures []string

	pairs := strings.Split(sigHeader, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			switch parts[0] {
			case "t":
				timestampStr = parts[1]
			case "v1":
				signatures = append(signatures, parts[1])
			}
		}
	}

	if timestampStr == "" || len(signatures) == 0 {
		return fmt.Errorf("Stripe-Signature 请求头格式无效")
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("无效的时间戳: %w", err)
	}

	if tolerance > 0 {
		now := time.Now().Unix()
		diff := now - ts
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(tolerance.Seconds()) {
			return fmt.Errorf("Webhook 时间戳超出允许容忍时间窗口 (可能为重放攻击)")
		}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestampStr))
	mac.Write([]byte("."))
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	matched := false
	for _, sig := range signatures {
		sigBytes, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(sigBytes, expectedSig) {
			matched = true
			break
		}
	}

	if !matched {
		return fmt.Errorf("Webhook 签名验证不通过")
	}

	return nil
}

// StripeWebhook processes Stripe payment webhook events.
func (h *AdminHandler) StripeWebhook(c *gin.Context) {
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	sigHeader := c.GetHeader("Stripe-Signature")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求体"})
		return
	}

	// Verify webhook signature in production or whenever secret/signature header is present
	if webhookSecret != "" || sigHeader != "" || gin.Mode() != gin.TestMode {
		if err := VerifyStripeWebhookSignature(body, sigHeader, webhookSecret, 5*time.Minute); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Stripe 签名校验失败: " + err.Error()})
			return
		}
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
			_, err := h.repo.CompleteRechargeOrder(orderNo)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "订单处理失败: " + err.Error()})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
