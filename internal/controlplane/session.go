package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
)

const (
	// SessionCookieName holds the signed admin token in an HttpOnly cookie.
	SessionCookieName = "airoute_session"
	// CSRFCookieName holds the double-submit CSRF token (readable by JS).
	CSRFCookieName = "airoute_csrf"
	// CSRFHeaderName must echo the CSRF cookie on cookie-authenticated writes.
	CSRFHeaderName = "X-CSRF-Token"
	// SessionTTLSeconds keeps the cookie aligned with the 7-day token lifetime.
	SessionTTLSeconds = 7 * 24 * 60 * 60
)

// cookieSecure decides whether the session cookies may carry the Secure flag.
//
// The decision is made from something the *gateway* can trust: TLS on the
// connection itself, or a forwarded scheme reported by a configured trusted proxy.
// A client-supplied X-Forwarded-Proto is ignored, because any caller can send one
// and the shipped nginx forwards the client's own value verbatim - trusting it let
// a caller have its seven-day session cookie issued without Secure, which a browser
// will then also send over plain http.
func cookieSecure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if c.Request != nil && c.Request.TLS != nil {
		return true
	}
	if proto, ok := middleware.TrustedForwardedProto(c); ok {
		return strings.EqualFold(proto, "https")
	}
	return false
}

// SetSessionCookies stores the session token in an HttpOnly cookie and issues a
// readable CSRF token for double-submit validation.
func SetSessionCookies(c *gin.Context, token string) {
	secure := cookieSecure(c)
	csrf := newCSRFToken()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   SessionTTLSeconds,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrf,
		Path:     "/",
		MaxAge:   SessionTTLSeconds,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookies expires both session cookies.
func ClearSessionCookies(c *gin.Context) {
	secure := cookieSecure(c)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// sessionTokenFromCookie returns the session token stored in the HttpOnly cookie.
func sessionTokenFromCookie(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if cookie, err := c.Cookie(SessionCookieName); err == nil {
		return strings.TrimSpace(cookie)
	}
	return ""
}

// CSRFValid reports whether a cookie-authenticated request carries the matching
// double-submit token. Safe methods (GET/HEAD/OPTIONS) are exempt.
func CSRFValid(c *gin.Context) bool {
	if c == nil {
		return false
	}
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	cookie, err := c.Cookie(CSRFCookieName)
	if err != nil || cookie == "" {
		return false
	}
	return strings.EqualFold(cookie, strings.TrimSpace(c.GetHeader(CSRFHeaderName)))
}

func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
