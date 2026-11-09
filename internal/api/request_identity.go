package api

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// getKeyGroup extracts key group name from context.
func getKeyGroup(c *gin.Context) string {
	if c == nil {
		return "default"
	}
	if kAny, exists := c.Get(middleware.ContextKeyAPIKeyConfig); exists {
		if k, ok := kAny.(*model.APIKeyConfig); ok && k.GroupName != "" {
			return k.GroupName
		}
	}
	return "default"
}

// getRequestAPIKey extracts incoming API key token string.
func getRequestAPIKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(middleware.ContextKeyAPIKey)
}

// getAPIKeyConfig returns APIKeyConfig metadata bound to the context.
func getAPIKeyConfig(c *gin.Context) *model.APIKeyConfig {
	if c == nil {
		return nil
	}
	if val, exists := c.Get(middleware.ContextKeyAPIKeyConfig); exists {
		if cfg, ok := val.(*model.APIKeyConfig); ok {
			return cfg
		}
	}
	return nil
}

// callerOwner identifies the caller that owns a stored object (a Responses
// conversation, a video task): the authenticated user when the API key belongs to
// one, otherwise the API-key digest. Two credentials must never share an object,
// so the owner string has to be stable across requests and must not collide
// between a user and a bare key.
func callerOwner(c *gin.Context) string {
	if value, ok := c.Get("native_job_owner"); ok {
		owner, _ := value.(string)
		return owner
	}
	key := getAPIKeyConfig(c)
	if key == nil || key.Key == "" {
		return ""
	}
	if key.UserID > 0 {
		return "user:" + strconv.FormatInt(key.UserID, 10)
	}
	digest := sha256.Sum256([]byte(key.Key))
	return "key:" + hex.EncodeToString(digest[:])
}
