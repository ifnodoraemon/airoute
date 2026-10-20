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

	state, err := newOAuthState(provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "无法生成 OAuth 安全状态参数"})
		return
	}
	setOAuthStateCookie(c, state)

	var authURL string
	if provider == "github" {
		authURL = fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=user:email&state=%s", clientID, url.QueryEscape(redirectURI), url.QueryEscape(state))
	} else {
		authURL = fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid%%20email%%20profile&state=%s", clientID, url.QueryEscape(redirectURI), url.QueryEscape(state))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"provider":   provider,
		"configured": true,
		"auth_url":   authURL,
		"state":      state,
	})
}

// OAuthCallbackRequest defines OAuth callback body.
type OAuthCallbackRequest struct {
	Code      string `json:"code"`
	State     string `json:"state"`
	Simulated bool   `json:"simulated"`
	Username  string `json:"username"`
	Email     string `json:"email"`
}

// oauthError replies with JSON for API callers and redirects browser (GET)
// callbacks back to the SPA, so a failed or successful login never strands the
// user on a raw JSON page.
func (h *AdminHandler) oauthError(c *gin.Context, status int, message string) {
	if c.Request.Method == http.MethodGet {
		h.redirectOAuthResult(c, "", message)
		return
	}
	c.JSON(status, gin.H{"code": status, "error": message})
}

// redirectOAuthResult sends the browser back to the console with either a
// short-lived, single-use exchange code or an error message. The session token
// itself is never placed in the URL.
func (h *AdminHandler) redirectOAuthResult(c *gin.Context, exchangeCode, errMsg string) {
	base := strings.TrimRight(ResolvePublicBaseURL(c), "/")
	q := url.Values{}
	if exchangeCode != "" {
		q.Set("oauth_code", exchangeCode)
	}
	if errMsg != "" {
		q.Set("oauth_error", errMsg)
	}
	target := base + "/#/login"
	if len(q) > 0 {
		target = base + "/?" + q.Encode() + "#/login"
	}
	c.Redirect(http.StatusFound, target)
}

// OAuthCallback handles GitHub/Google OAuth login or 1-click test authorization.
func (h *AdminHandler) OAuthCallback(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	if provider != "github" && provider != "google" {
		h.oauthError(c, http.StatusBadRequest, "不支持的 OAuth 提供商")
		return
	}

	var req OAuthCallbackRequest
	_ = c.ShouldBindJSON(&req)

	// Code and state may arrive via query string on the browser redirect.
	if req.Code == "" {
		req.Code = c.Query("code")
	}
	stateParam := req.State
	if stateParam == "" {
		stateParam = c.Query("state")
	}

	clientID := os.Getenv(fmt.Sprintf("%s_CLIENT_ID", strings.ToUpper(provider)))
	clientSecret := os.Getenv(fmt.Sprintf("%s_CLIENT_SECRET", strings.ToUpper(provider)))
	redirectURI := os.Getenv(fmt.Sprintf("%s_REDIRECT_URI", strings.ToUpper(provider)))
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/auth/oauth/%s/callback", ResolvePublicBaseURL(c), provider)
	}

	isTestOrDev := strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/") || strings.EqualFold(os.Getenv("ENABLE_SIMULATED_OAUTH"), "true")

	var email, username string
	realExchange := false

	if clientID != "" && clientSecret != "" && req.Code != "" && req.Code != "demo" && !req.Simulated {
		// Real production token exchange: the OAuth state must be valid, signed
		// for this provider, unexpired, and bound to this browser. This closes
		// the OAuth login-CSRF hole (an attacker-supplied code cannot be
		// replayed into a victim's session).
		if err := bindOAuthState(c, stateParam, provider); err != nil {
			h.oauthError(c, http.StatusForbidden, "OAuth 安全校验失败: "+err.Error())
			return
		}
		clearOAuthStateCookie(c)
		realExchange = true

		if provider == "github" {
			realUser, realEmail, err := exchangeGitHubOAuth(req.Code, clientID, clientSecret)
			if err != nil {
				h.oauthError(c, http.StatusBadRequest, "GitHub 授权失败: "+err.Error())
				return
			}
			username = "gh_" + realUser
			email = realEmail
		} else {
			realUser, realEmail, err := exchangeGoogleOAuth(req.Code, clientID, clientSecret, redirectURI)
			if err != nil {
				h.oauthError(c, http.StatusBadRequest, "Google 授权失败: "+err.Error())
				return
			}
			username = "gg_" + realUser
			email = realEmail
		}
	} else if isTestOrDev {
		// Simulated authentication is only reachable in test binaries or when
		// ENABLE_SIMULATED_OAUTH=true is set explicitly. It must never be able to
		// take over a privileged account, so the requested identity is only
		// honoured for newly generated demo users.
		if req.Username != "" {
			existing, _ := h.repo.GetUserByUsername(req.Username)
			if existing != nil {
				h.oauthError(c, http.StatusForbidden, "模拟登录不能接管已存在的账号，请使用标准密码或真实 OAuth 登录")
				return
			}
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
		h.oauthError(c, http.StatusForbidden, fmt.Sprintf("系统未配置 %s 生产应用密钥，禁止使用模拟快捷登录", strings.ToUpper(provider)))
		return
	}

	// Find or create user. Usernames are looked up first, then emails; both use
	// explicit lookups so an email can never be mistaken for a username.
	user, _ := h.repo.GetUserByUsername(username)
	if user == nil && email != "" {
		user, _ = h.repo.GetUserByEmail(email)
	}

	// Security: a simulated/demo authorization may never log in to an existing
	// administrator account, even if the caller forged an arbitrary code.
	if !realExchange && user != nil && strings.EqualFold(user.Role, "admin") {
		h.oauthError(c, http.StatusForbidden, "管理员账号不允许通过模拟演示登录，请使用标准密码登录")
		return
	}

	if user == nil {
		// New OAuth user: create with trial balance
		randPassBytes := make([]byte, 16)
		if _, err := rand.Read(randPassBytes); err != nil {
			h.oauthError(c, http.StatusInternalServerError, "安全随机源不可用")
			return
		}
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
		if err := h.repo.CreateUser(user); err != nil {
			h.oauthError(c, http.StatusInternalServerError, "创建 OAuth 账户失败: "+err.Error())
			return
		}

		// Create default API key
		keyBytes := make([]byte, 16)
		if _, err := rand.Read(keyBytes); err != nil {
			h.oauthError(c, http.StatusInternalServerError, "安全随机源不可用，无法生成密钥")
			return
		}
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
		h.oauthError(c, http.StatusForbidden, "该账户已被管理员锁定，无法登录")
		return
	}

	token, err := GenerateAdminToken(user.Username, user.Role, 7*24*time.Hour)
	if err != nil {
		h.oauthError(c, http.StatusInternalServerError, "生成鉴权 Token 失败")
		return
	}
	SetSessionCookies(c, token)

	// Browser flow: hand the SPA a one-time code instead of rendering JSON.
	if c.Request.Method == http.MethodGet {
		exchangeCode, err := issueOAuthExchangeCode(token, user)
		if err != nil {
			h.oauthError(c, http.StatusInternalServerError, "生成授权交换码失败")
			return
		}
		h.redirectOAuthResult(c, exchangeCode, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("通过 %s 授权登录成功", strings.ToUpper(provider)),
		"data": gin.H{
			"token": token,
			"user":  oauthUserPayload(user),
		},
	})
}
