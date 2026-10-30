package controlplane

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// Cross-site request defences for the unauthenticated auth endpoints.
//
// /api/v1/auth/login, /api/v1/auth/register and /api/v1/auth/logout are the only
// state-changing routes served without a session, so they are the only ones the
// session middleware's CSRF double-submit cannot protect. Cookies are
// SameSite=Lax, which governs whether a browser SENDS a cookie on a cross-site
// request; it does not stop a cross-site response from SETTING or CLEARING one.
// A cross-site top-level form POST can therefore:
//
//   - log the victim into the attacker's account (login CSRF / session swap: the
//     console, and every recharge or created key after it, belongs to the
//     attacker), or
//   - clear the victim's session (logout CSRF).
//
// Two independent gates close that, both cheap and both invisible to real API
// clients:
//
//  1. the request must carry Content-Type: application/json. A cross-site HTML
//     form can only send urlencoded/multipart/text-plain, and the classic
//     "text/plain body shaped like JSON" trick relies on the handler parsing JSON
//     regardless of the content type. Browsers do not let a page send a JSON
//     content type cross-site without a CORS preflight, which these routes do not
//     answer.
//  2. when the browser does send Origin (or, failing that, Referer), it must name
//     this origin. CLI and scripted clients send neither header and keep working.
//
// See BUG-CP-05 in docs/audit/2026-10-29-bughunt/01-controlplane.md (carried over
// from the previous round's C-05, whose "SameSite=Lax mitigates it" conclusion
// was wrong).

// requireJSONBody enforces the JSON content type on an unauthenticated
// state-changing endpoint, writing 415 otherwise.
func requireJSONBody(c *gin.Context) bool {
	if !isJSONContentType(c.GetHeader("Content-Type")) {
		c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{
			"code":  415,
			"error": "请求必须使用 Content-Type: application/json",
		})
		return false
	}
	return true
}

// isJSONContentType reports whether the header names JSON, ignoring parameters
// such as "; charset=utf-8".
func isJSONContentType(header string) bool {
	if i := strings.IndexByte(header, ';'); i >= 0 {
		header = header[:i]
	}
	return strings.EqualFold(strings.TrimSpace(header), "application/json")
}

// requireSameOrigin rejects a request whose Origin/Referer names a different site,
// writing 403 otherwise. Absent headers mean a non-browser client and are allowed.
func requireSameOrigin(c *gin.Context) bool {
	if err := sameOriginError(c); err != "" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":  403,
			"error": "请求来源校验失败，请从网关控制台页面重试",
		})
		return false
	}
	return true
}

// sameOriginError returns a non-empty reason when the request's Origin (or
// Referer) does not match this gateway.
func sameOriginError(c *gin.Context) string {
	raw := strings.TrimSpace(c.GetHeader("Origin"))
	if raw == "" {
		if referer := strings.TrimSpace(c.GetHeader("Referer")); referer != "" {
			if u, err := url.Parse(referer); err == nil && u.Host != "" {
				raw = u.Scheme + "://" + u.Host
			}
		}
	}
	if raw == "" {
		// No browser-supplied origin: a CLI, SDK or server-side client. There is no
		// ambient credential to abuse on these endpoints, so allow it.
		return ""
	}
	origin, err := url.Parse(raw)
	if err != nil || origin.Host == "" {
		// Includes the literal "Origin: null" sent by sandboxed documents and
		// file:// pages: those are exactly the cross-site cases we must refuse.
		return "无法解析的请求来源"
	}
	if requestHostMatches(c, origin.Host) {
		return ""
	}
	return "请求来源与网关不一致"
}

// requestHostMatches reports whether candidateHost is one of the origins this
// gateway may legitimately be reached on: the configured public URL (authoritative
// when set) or the Host the request itself was addressed to.
func requestHostMatches(c *gin.Context, candidateHost string) bool {
	if cfg := config.GetGlobalConfig(); cfg != nil {
		if pub := cfg.GetPublicURL(); pub != "" {
			if u, err := url.Parse(pub); err == nil && u.Host != "" && strings.EqualFold(candidateHost, u.Host) {
				return true
			}
			// A configured public URL is authoritative: do not also accept an
			// attacker-chosen Host header once the operator has named the origin.
			return false
		}
	}
	if c != nil && c.Request != nil && c.Request.Host != "" {
		return strings.EqualFold(candidateHost, c.Request.Host)
	}
	return false
}
