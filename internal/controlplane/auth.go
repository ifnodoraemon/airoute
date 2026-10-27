package controlplane

import (
	"crypto/hmac"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetClaimsFromContext safely extracts verified AdminClaims from the gin context.
func GetClaimsFromContext(c *gin.Context) (*AdminClaims, bool) {
	if c == nil {
		return nil, false
	}
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		return nil, false
	}
	claims, ok := claimsVal.(*AdminClaims)
	return claims, ok && claims != nil
}

// RequireAuthClaims validates that the request has an authenticated session, writing a 401 response if absent.
func RequireAuthClaims(c *gin.Context) (*AdminClaims, bool) {
	claims, ok := GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "请先登录账号"})
		return nil, false
	}
	return claims, true
}

// RequireAdminClaims validates that the request has super-admin privileges, writing 401 or 403 responses if not.
func RequireAdminClaims(c *gin.Context, forbiddenMsg string) (*AdminClaims, bool) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return nil, false
	}
	if claims.Role != "admin" {
		if forbiddenMsg == "" {
			forbiddenMsg = "权限不足，仅超级管理员可执行此操作"
		}
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": forbiddenMsg})
		return nil, false
	}
	return claims, true
}

// AdminAuthMiddleware validates the admin bearer token.
func (h *AdminHandler) AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow login endpoint
		if strings.HasSuffix(c.Request.URL.Path, "/auth/login") {
			c.Next()
			return
		}

		// Credentials are accepted from headers and the session cookie only.
		//
		// A ?token= fallback used to exist here for EventSource callers that cannot
		// set headers. It put a 7-day session credential into URLs, where it leaks
		// through access logs, browser history and Referer headers; the console now
		// authenticates the log stream with the HttpOnly cookie instead.
		authHeader := c.GetHeader("Authorization")
		var token string
		cookieAuth := false
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if xToken := c.GetHeader("x-admin-token"); xToken != "" {
			token = xToken
		} else if cookieToken := sessionTokenFromCookie(c); cookieToken != "" {
			token = cookieToken
			cookieAuth = true
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "请先登录管理员账号 (未提供凭证)"})
			return
		}

		// Cookie-authenticated writes additionally require the double-submit
		// CSRF token; Bearer/header clients are immune to CSRF by construction.
		if cookieAuth && !CSRFValid(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "error": "CSRF 校验失败，请刷新页面后重试"})
			return
		}

		claims, err := VerifyAdminToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "登录凭证已过期或无效: " + err.Error()})
			return
		}

		if h.repo != nil {
			user, err := h.repo.GetUserByUsername(claims.Username)
			if err != nil || user == nil || user.Status == "deleted" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "账户不存在或暂时无法验证，请重新登录"})
				return
			}
			if strings.EqualFold(user.Status, "locked") {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "error": "该账户已被管理员锁定，无法继续访问"})
				return
			}
			if !hmac.Equal([]byte(claims.CredentialTag), []byte(sessionCredentialTag(user))) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "登录凭证已撤销，请重新登录"})
				return
			}
			// A role change invalidates previously issued tokens immediately.
			if user.Role != claims.Role {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "账户权限已变更，请重新登录"})
				return
			}
		}

		c.Set("admin_claims", claims)
		c.Set("admin_username", claims.Username)
		c.Next()
	}
}

// RequireAdminRole verifies that the authenticated user possesses the admin role.
func (h *AdminHandler) RequireAdminRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := GetClaimsFromContext(c)
		if !ok || claims.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "error": "权限不足，仅超级管理员可执行此操作"})
			return
		}
		c.Next()
	}
}
