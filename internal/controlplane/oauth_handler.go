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

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
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
