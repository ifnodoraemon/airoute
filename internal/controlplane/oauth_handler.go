package controlplane

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
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

// oauthRedirectBase returns the origin the post-login redirect may point at.
//
// It only ever returns an operator-configured public URL. The previous code
// derived it from ResolvePublicBaseURL, which falls back to the request's Host
// and X-Forwarded-Host headers — both chosen by the caller — so an attacker
// could make a successful login redirect the victim to their own origin. When
// no public URL is configured the caller redirects to a relative path instead,
// which stays on whatever origin served the callback.
func oauthRedirectBase() string {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return ""
	}
	// GetPublicURL already prefers PUBLIC_URL/GATEWAY_PUBLIC_URL, validates the
	// scheme and trims the trailing slash.
	return cfg.GetPublicURL()
}

// redirectOAuthResult returns to the SPA after setting the session cookie.
// The URL carries only a completion marker or an error, never credentials.
func (h *AdminHandler) redirectOAuthResult(c *gin.Context, completed, errMsg string) {
	q := url.Values{}
	if completed != "" {
		q.Set("oauth_complete", "1")
	}
	if errMsg != "" {
		q.Set("oauth_error", errMsg)
	}
	target := "/#/login"
	if len(q) > 0 {
		target = "/?" + q.Encode() + "#/login"
	}
	// Configured deployments get an absolute URL; unconfigured ones stay on the
	// serving origin rather than trusting a request header.
	if base := oauthRedirectBase(); base != "" {
		target = base + target
	}
	c.Redirect(http.StatusFound, target)
}

// OAuth account usernames are a namespace of the gateway, but the value is
// derived from public provider identifiers (a GitHub numeric id, a Google sub),
// so anyone can register the same name locally before that person first signs in.
// The base name is therefore only a preference: allocation appends a random suffix
// until it finds a free name instead of failing the login with a 500.
const (
	oauthUsernameMaxLength   = 64
	oauthUsernameMaxAttempts = 8
	oauthUsernameSuffixLen   = 4
)

// oauthUsernameBase builds the preferred username for an identity. The subject is
// sanitized to characters that are safe in a username; anything else is dropped so
// a provider value can never smuggle routing or display characters into the name.
func oauthUsernameBase(provider, subject string) string {
	prefix := "gh_"
	if strings.EqualFold(provider, "google") {
		prefix = "gg_"
	}
	var b strings.Builder
	for _, r := range subject {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	sanitized := b.String()
	if sanitized == "" {
		// A provider that returned nothing usable still gets a stable, unique name
		// derived from the subject so the account is not shared between identities.
		sum := sha256.Sum256([]byte(provider + "\x00" + subject))
		sanitized = hex.EncodeToString(sum[:6])
	}
	if len(sanitized) > oauthUsernameMaxLength-len(prefix)-oauthUsernameSuffixLen-1 {
		sanitized = sanitized[:oauthUsernameMaxLength-len(prefix)-oauthUsernameSuffixLen-1]
	}
	return prefix + sanitized
}

// allocateOAuthUsername returns a free username for a new OAuth account. The
// preferred name is used when free; otherwise a short random suffix is appended
// (and retried), so a squatted name degrades to a different valid name instead of
// blocking the owner's first login.
func allocateOAuthUsername(repo *storage.Repository, provider, subject, preferred string) (string, error) {
	base := strings.TrimSpace(preferred)
	if base == "" {
		base = oauthUsernameBase(provider, subject)
	}
	for attempt := 0; attempt < oauthUsernameMaxAttempts; attempt++ {
		candidate := base
		if attempt > 0 {
			suffix, err := randomUsernameSuffix()
			if err != nil {
				return "", err
			}
			candidate = base + "_" + suffix
		}
		existing, err := repo.GetUserByUsername(candidate)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		if existing == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate a free username for the OAuth identity after %d attempts", oauthUsernameMaxAttempts)
}

// randomUsernameSuffix returns a short hex suffix from the system entropy source.
func randomUsernameSuffix() (string, error) {
	buf := make([]byte, oauthUsernameSuffixLen/2)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
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

	var email, username, subject string
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
			subject = realUser
			// The username is allocated when the account is created: the obvious
			// gh_<id> name can be squatted by local registration.
			username = ""
			email = realEmail
		} else {
			realUser, realEmail, err := exchangeGoogleOAuth(req.Code, clientID, clientSecret, redirectURI)
			if err != nil {
				h.oauthError(c, http.StatusBadRequest, "Google 授权失败: "+err.Error())
				return
			}
			subject = realUser
			username = ""
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

	var user *storage.UserRecord
	var lookupErr error
	if realExchange {
		user, lookupErr = h.repo.GetUserByOAuthIdentity(provider, subject)
	} else {
		user, lookupErr = h.repo.GetUserByUsername(username)
	}
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		h.oauthError(c, http.StatusInternalServerError, "无法读取账号，请稍后重试")
		return
	}
	if !realExchange && user == nil && email != "" {
		user, lookupErr = h.repo.GetUserByEmail(email)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			h.oauthError(c, 500, "无法验证邮箱")
			return
		}
	}
	if !realExchange && user != nil {
		h.oauthError(c, http.StatusForbidden, "模拟登录不能接管已有账号")
		return
	}

	if user == nil && realExchange && email != "" {
		owner, err := h.repo.GetUserByEmail(email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			h.oauthError(c, 500, "无法验证邮箱")
			return
		}
		if owner != nil {
			h.oauthError(c, http.StatusConflict, "该邮箱已有账号，请使用原账号登录；第三方身份不会自动关联")
			return
		}
	}
	if user == nil {
		if !config.GetGlobalConfig().IsRegistrationAllowed() {
			h.oauthError(c, http.StatusForbidden, "自主注册已关闭，请联系管理员分配账号")
			return
		}
		// New OAuth user: create with trial balance
		randPassBytes := make([]byte, 16)
		if _, err := rand.Read(randPassBytes); err != nil {
			h.oauthError(c, http.StatusInternalServerError, "安全随机源不可用")
			return
		}
		hash, err := bcrypt.GenerateFromPassword(randPassBytes, bcrypt.DefaultCost)
		if err != nil {
			h.oauthError(c, http.StatusInternalServerError, "创建 OAuth 账户失败")
			return
		}

		allocated, allocErr := allocateOAuthUsername(h.repo, provider, subject, username)
		if allocErr != nil {
			h.oauthError(c, http.StatusInternalServerError, "无法为第三方账号分配用户名，请稍后重试")
			return
		}
		username = allocated
		user = &storage.UserRecord{
			Username:     username,
			Email:        email,
			PasswordHash: string(hash),
			Role:         "user",
			Status:       "active",
			Balance:      config.GetGlobalConfig().GetInitialUserBalance(),
			GroupName:    "default",
		}
		// Create default API key
		newKey, err := GenerateAPIKey()
		if err != nil {
			h.oauthError(c, http.StatusInternalServerError, "安全随机源不可用，无法生成密钥")
			return
		}
		if err := h.repo.CreateOAuthUserWithAPIKey(provider, subject, user, &storage.APIKeyRecord{
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
			h.oauthError(c, http.StatusInternalServerError, "创建 OAuth 账号及初始密钥失败，请重试")
			return
		}
		h.syncDataPlane()
	}

	if strings.EqualFold(user.Status, "locked") || strings.EqualFold(user.Status, "deleted") {
		h.oauthError(c, http.StatusForbidden, "该账户已被锁定或删除，无法登录")
		return
	}

	token, err := GenerateAdminToken(user, 7*24*time.Hour)
	if err != nil {
		h.oauthError(c, http.StatusInternalServerError, "生成鉴权 Token 失败")
		return
	}
	SetSessionCookies(c, token)

	// The callback already installed the HttpOnly session. Any replica can
	// authenticate it; the SPA does not exchange a process-local code.
	if c.Request.Method == http.MethodGet {
		h.redirectOAuthResult(c, "1", "")
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
