package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

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
