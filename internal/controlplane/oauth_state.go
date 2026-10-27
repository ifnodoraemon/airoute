package controlplane

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

const (
	// OAuthStateCookieName binds an OAuth authorization request to the browser
	// that started it (double-submit), on top of the signed state parameter.
	OAuthStateCookieName = "airoute_oauth_state"
	oauthStateTTL        = 10 * time.Minute
)

// oauthStateKey derives a dedicated signing key so an OAuth state can never be
// confused with, or forged from, an admin session token.
func oauthStateKey() []byte {
	mac := hmac.New(sha256.New, getAdminSecret())
	mac.Write([]byte("airoute-oauth-state-v1"))
	return mac.Sum(nil)
}

// newOAuthState returns a signed, self-expiring state value for the provider.
func newOAuthState(provider string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%s|%d|%s", strings.ToLower(provider), time.Now().Unix(), hex.EncodeToString(nonce))
	mac := hmac.New(sha256.New, oauthStateKey())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(mac.Sum(nil)), nil
}

// verifyOAuthState checks the state signature, provider binding and age.
func verifyOAuthState(state, provider string) error {
	state = strings.TrimSpace(state)
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return fmt.Errorf("缺少或格式错误的 OAuth state 参数")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("格式错误的 OAuth state 参数")
	}
	mac := hmac.New(sha256.New, oauthStateKey())
	mac.Write(raw)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) != 1 {
		return fmt.Errorf("OAuth state 签名校验失败")
	}

	fields := strings.Split(string(raw), "|")
	if len(fields) != 3 {
		return fmt.Errorf("格式错误的 OAuth state 载荷")
	}
	if !strings.EqualFold(fields[0], provider) {
		return fmt.Errorf("OAuth state 与授权提供商不匹配")
	}
	issued, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return fmt.Errorf("格式错误的 OAuth state 时间戳")
	}
	if age := time.Since(time.Unix(issued, 0)); age > oauthStateTTL || age < -time.Minute {
		return fmt.Errorf("OAuth state 已过期，请重新发起登录")
	}
	return nil
}

func setOAuthStateCookie(c *gin.Context, state string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   cookieSecure(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthStateCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookieSecure(c),
		SameSite: http.SameSiteLaxMode,
	})
}

// bindOAuthState validates the returned state against both its signature and
// the cookie issued by OAuthInitiate, which defeats OAuth login CSRF (an
// attacker's callback cannot be replayed into a victim's browser).
func bindOAuthState(c *gin.Context, state, provider string) error {
	if err := verifyOAuthState(state, provider); err != nil {
		return err
	}
	cookieState, _ := c.Cookie(OAuthStateCookieName)
	if cookieState == "" || subtle.ConstantTimeCompare([]byte(cookieState), []byte(state)) != 1 {
		return fmt.Errorf("OAuth state 与当前浏览器会话不匹配，请重新发起登录")
	}
	return nil
}

// oauthUserPayload renders the non-sensitive user fields returned to the SPA.
func oauthUserPayload(u *storage.UserRecord) gin.H {
	if u == nil {
		return gin.H{}
	}
	return gin.H{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"role":       u.Role,
		"status":     u.Status,
		"balance":    u.Balance,
		"group_name": u.GroupName,
	}
}
