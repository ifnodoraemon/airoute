package controlplane

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListRequestTraces returns redacted request snapshots for troubleshooting.
// Super-admins can search any key; regular users are scoped to their own keys.
func (h *AdminHandler) ListRequestTraces(c *gin.Context) {
	filter := storage.TraceFilter{
		TraceID:   strings.TrimSpace(c.Query("trace_id")),
		ChatID:    strings.TrimSpace(c.Query("chat_id")),
		SessionID: strings.TrimSpace(c.Query("session_id")),
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}

	if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
		user, err := h.repo.GetUserByUsername(claims.Username)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "账户不存在或暂时无法验证，请重新登录"})
			return
		}
		// Historical keys: a revoked key's traces still belong to its owner. Using
		// the live list here hid a user's own trace history as soon as the key that
		// produced it was deleted (admin_logs.go uses the historical list for the
		// same reason).
		keys, err := h.repo.ListHistoricalAPIKeysByUser(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(keys) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": []any{}})
			return
		}
		// Scope to every key the user owns, using an IN clause so a user with
		// multiple keys still sees their complete trace history.
		filter.APIKeys = make([]string, 0, len(keys))
		for _, k := range keys {
			filter.APIKeys = append(filter.APIKeys, k.Key)
		}
	}

	traces, err := h.repo.ListRequestTraces(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if traces == nil {
		traces = []*storage.RequestTraceRecord{}
	}
	h.presentTraceKeys(traces)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": traces})
}

// presentTraceKeys names archived traces the way operators identify keys: the
// stored digest becomes the public identifier, and an unresolved digest is masked
// instead of being returned raw.
func (h *AdminHandler) presentTraceKeys(traces []*storage.RequestTraceRecord) {
	if len(traces) == 0 {
		return
	}
	digests := make([]string, 0, len(traces))
	for _, t := range traces {
		if t != nil && t.APIKey != "" && !storage.IsAPIKeyPublicID(t.APIKey) {
			digests = append(digests, t.APIKey)
		}
	}
	ids, _ := h.repo.APIKeyPublicIDsByDigest(digests)
	for _, t := range traces {
		if t == nil || t.APIKey == "" || storage.IsAPIKeyPublicID(t.APIKey) {
			continue
		}
		if publicID, ok := ids[t.APIKey]; ok && publicID != "" {
			t.APIKey = publicID
			continue
		}
		t.APIKey = maskAPIKey(t.APIKey)
	}
}
