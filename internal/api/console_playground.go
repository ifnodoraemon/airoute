package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

const consolePlaygroundPrefix = "/api/v1/playground"

// Console calls use the existing HttpOnly login session and a non-secret row ID.
// The public SDK endpoints continue to require bearer credentials. No secret is
// recovered, minted or sent to the browser for an interactive console call.
func registerConsolePlaygroundRoutes(r *gin.Engine, admin *controlplane.AdminHandler, h *Handler, mm *MultimodalHandler) {
	console := r.Group(consolePlaygroundPrefix)
	console.Use(admin.AdminAuthMiddleware())
	console.Use(func(c *gin.Context) {
		// Existing handlers and admission rules operate on canonical SDK paths
		// (including free video-task reads and owned Gemini media downloads).
		// Restore the console path before the outer access logger completes.
		path, rawPath := c.Request.URL.Path, c.Request.URL.RawPath
		c.Request.URL.Path = strings.TrimPrefix(path, consolePlaygroundPrefix)
		c.Request.URL.RawPath = strings.TrimPrefix(rawPath, consolePlaygroundPrefix)
		defer func() { c.Request.URL.Path, c.Request.URL.RawPath = path, rawPath }()
		c.Next()
	})
	console.Use(middleware.SessionKeyAuthMiddleware(consoleKeyResolver(admin)))
	console.Use(middleware.RateLimitMiddleware())
	registerDataPlaneRoutes(console.Group("/v1"), h, mm)
	gemini := console.Group("/v1beta")
	gemini.GET("/models", h.HandleGeminiModels)
	gemini.GET("/models/*modelAction", h.HandleGeminiModelDetail)
	gemini.POST("/models/*modelAction", h.HandleGeminiAction)
	for _, collection := range []string{"files", "generatedFiles"} {
		gemini.GET("/"+collection+"/*resource", h.HandleGeminiResource)
	}
}

func consoleKeyResolver(admin *controlplane.AdminHandler) func(*gin.Context) (*model.APIKeyConfig, *middleware.AccessDenial) {
	return func(c *gin.Context) (*model.APIKeyConfig, *middleware.AccessDenial) {
		deny := func(status int, code, message string) (*model.APIKeyConfig, *middleware.AccessDenial) {
			return nil, &middleware.AccessDenial{StatusCode: status, Type: "permission_error", Code: code, Message: message}
		}
		claims, ok := controlplane.GetClaimsFromContext(c)
		if !ok {
			return deny(http.StatusUnauthorized, "session_required", "Please sign in to use the playground.")
		}
		id, err := strconv.ParseInt(c.GetHeader("X-Airoute-Key-ID"), 10, 64)
		if err != nil || id <= 0 {
			return deny(http.StatusBadRequest, "key_selection_required", "Select an API key before sending a request.")
		}
		repo := admin.GetRepo()
		if repo == nil {
			return deny(http.StatusServiceUnavailable, "key_state_unavailable", "Key state is temporarily unavailable. Please retry.")
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		key, err := repo.GetAPIKeyContext(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return deny(http.StatusForbidden, "key_access_denied", "The selected key is unavailable or you do not have access to it.")
		}
		if err != nil || key == nil {
			return deny(http.StatusServiceUnavailable, "key_state_unavailable", "Key state is temporarily unavailable. Please retry.")
		}
		if claims.Role != "admin" {
			userID := controlplane.GetSessionUserID(c)
			if userID <= 0 || key.UserID != userID {
				return deny(http.StatusForbidden, "key_access_denied", "The selected key is unavailable or you do not have access to it.")
			}
		}
		if denial := middleware.KeyStatusDenial(key.Status); denial != nil {
			// Revoking a selected key does not invalidate the login session.
			denial.StatusCode = http.StatusForbidden
			return nil, denial
		}
		cfg := key.ToConfig()
		return &cfg, nil
	}
}
