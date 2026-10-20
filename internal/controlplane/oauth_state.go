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
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

const (
	// OAuthStateCookieName binds an OAuth authorization request to the browser
	// that started it (double-submit), on top of the signed state parameter.
	OAuthStateCookieName = "airoute_oauth_state"
	oauthStateTTL        = 10 * time.Minute
	oauthExchangeTTL     = 60 * time.Second
)

// oauthStateKey derives a dedicated signing key so an OAuth state can never be
// confused with, or forged from, an admin session token.
var oauthStateKey = func() []byte {
	mac := hmac.New(sha256.New, adminSecret)
	mac.Write([]byte("airoute-oauth-state-v1"))
	return mac.Sum(nil)
}()

// newOAuthState returns a signed, self-expiring state value for the provider.
func newOAuthState(provider string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%s|%d|%s", strings.ToLower(provider), time.Now().Unix(), hex.EncodeToString(nonce))
	mac := hmac.New(sha256.New, oauthStateKey)
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
	mac := hmac.New(sha256.New, oauthStateKey)
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
	if time.Since(time.Unix(issued, 0)) > oauthStateTTL {
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

// oauthExchangeEntry is a short-lived, single-use code that the browser
// callback redirect can hand to the SPA without putting the session token
// itself in the URL (where it would leak via history and Referer headers).
type oauthExchangeEntry struct {
	token     string
	user      *storage.UserRecord
	expiresAt time.Time
}

var oauthExchangeCodes sync.Map // code -> *oauthExchangeEntry

func issueOAuthExchangeCode(token string, user *storage.UserRecord) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := hex.EncodeToString(b)
	now := time.Now()
	oauthExchangeCodes.Range(func(k, v any) bool {
		if e, ok := v.(*oauthExchangeEntry); ok && now.After(e.expiresAt) {
			oauthExchangeCodes.Delete(k)
		}
		return true
	})
	oauthExchangeCodes.Store(code, &oauthExchangeEntry{token: token, user: user, expiresAt: now.Add(oauthExchangeTTL)})
	return code, nil
}

func consumeOAuthExchangeCode(code string) (*oauthExchangeEntry, bool) {
	if strings.TrimSpace(code) == "" {
		return nil, false
	}
	v, ok := oauthExchangeCodes.LoadAndDelete(code)
	if !ok {
		return nil, false
	}
	entry, ok := v.(*oauthExchangeEntry)
	if !ok || entry == nil || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry, true
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

// OAuthExchange swaps a one-time redirect code for the session. The session
// cookie is issued here as well, so the SPA never needs the raw token.
func (h *AdminHandler) OAuthExchange(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	_ = c.ShouldBindJSON(&req)
	code := strings.TrimSpace(req.Code)
	if code == "" {
		code = strings.TrimSpace(c.Query("code"))
	}
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "缺少授权交换码"})
		return
	}

	entry, ok := consumeOAuthExchangeCode(code)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "授权交换码无效或已过期，请重新登录"})
		return
	}
	if entry.user == nil || strings.EqualFold(entry.user.Status, "locked") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "该账户已被管理员锁定，无法登录"})
		return
	}

	SetSessionCookies(c, entry.token)
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "OAuth 登录成功",
		"data": gin.H{
			"token": entry.token,
			"user":  oauthUserPayload(entry.user),
		},
	})
}
