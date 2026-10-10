package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// extractSessionID retrieves explicit LLM session affinity key from HTTP headers, cookies, query, or payload user.
func extractSessionID(c *gin.Context, userField string) string {
	if s := c.GetHeader("X-Session-ID"); s != "" {
		return strings.TrimSpace(s)
	}
	if s := c.GetHeader("X-Conversation-ID"); s != "" {
		return strings.TrimSpace(s)
	}
	if s := c.GetHeader("Session-Id"); s != "" {
		return strings.TrimSpace(s)
	}
	if s := c.GetHeader("Conversation-Id"); s != "" {
		return strings.TrimSpace(s)
	}
	if cookie, err := c.Cookie("airoute_session"); err == nil && cookie != "" {
		return strings.TrimSpace(cookie)
	}
	if cookie, err := c.Cookie("nano_session"); err == nil && cookie != "" {
		return strings.TrimSpace(cookie)
	}
	if cookie, err := c.Cookie("session_id"); err == nil && cookie != "" {
		return strings.TrimSpace(cookie)
	}
	if s := c.Query("session_id"); s != "" {
		return strings.TrimSpace(s)
	}
	if userField != "" {
		return strings.TrimSpace(userField)
	}
	return ""
}

// deriveContextFingerprint computes a deterministic conversation anchor (system prompt + first user message)
// and generates a SHA-256 fingerprint bound to the client tenant/IP.
// This delivers 100% zero-touch, client-transparent session affinity across all conversational turns.
func deriveContextFingerprint(c *gin.Context, modelName string, messages []model.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	var firstUserMsg string
	var sysMsg string
	for _, m := range messages {
		if m.Role == "system" && sysMsg == "" {
			sysMsg = strings.TrimSpace(m.GetContentString())
		}
		if m.Role == "user" && firstUserMsg == "" {
			firstUserMsg = strings.TrimSpace(m.GetContentString())
			break
		}
	}
	if firstUserMsg == "" && sysMsg == "" {
		firstUserMsg = strings.TrimSpace(messages[0].GetContentString())
	}
	if firstUserMsg == "" && sysMsg == "" {
		return ""
	}

	if len(firstUserMsg) > 256 {
		firstUserMsg = firstUserMsg[:256]
	}
	if len(sysMsg) > 128 {
		sysMsg = sysMsg[:128]
	}

	tenant := c.GetString("tenant_id")
	if tenant == "" {
		tenant = getRequestAPIKey(c)
	}
	if tenant == "" {
		tenant = c.ClientIP()
	}

	data := fmt.Sprintf("%s|%s|%s|%s", tenant, modelName, sysMsg, firstUserMsg)
	h := sha256.Sum256([]byte(data))
	return "ctx_" + hex.EncodeToString(h[:12])
}

// resolveSessionID resolves either an explicit session ID or derives a zero-touch context fingerprint.
// It also writes sticky session headers and cookies onto the HTTP response.
func resolveSessionID(c *gin.Context, modelName string, messages []model.ChatMessage, userField string) string {
	sID := extractSessionID(c, userField)
	if sID == "" {
		sID = deriveContextFingerprint(c, modelName, messages)
	}
	if sID == "" {
		sID = fmt.Sprintf("sess_%d_%x", time.Now().Unix(), time.Now().UnixNano()%1000000)
	}
	if sID != "" {
		c.Header("X-Airoute-Session-ID", sID)
		// Set cookie for browser-based clients (NextChat, OpenWebUI, LibreChat, Web App)
		c.SetCookie("airoute_session", sID, 1800, "/", "", false, false)
		c.SetCookie("nano_session", sID, 1800, "/", "", false, false)
	}
	return sID
}

// resolveGenericSessionID extracts sticky session identifiers or derives a unique fallback session.
func resolveGenericSessionID(c *gin.Context, prefix string) string {
	sID := c.GetHeader("X-Session-ID")
	if sID == "" {
		sID = c.GetHeader("X-Airoute-Session-ID")
	}
	if sID == "" {
		if cookie, err := c.Cookie("airoute_session"); err == nil && cookie != "" {
			sID = cookie
		}
	}
	if sID == "" {
		if prefix == "" {
			prefix = "gen"
		}
		sID = fmt.Sprintf("sess_%s_%d_%x", prefix, time.Now().Unix(), time.Now().UnixNano()%1000000)
	}
	c.Header("X-Airoute-Session-ID", sID)
	return sID
}
