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

// llmSessionCookie is the data-plane sticky-session cookie used for LLM
// affinity. It MUST NOT be named "airoute_session": that name belongs to the
// control-plane login session (controlplane.SessionCookieName, HttpOnly,
// contains the admin JWT). Sharing the name made every inference call overwrite
// the console login cookie, and made the console JWT leak into usage_logs /
// Redis keys through the session affinity path.
const llmSessionCookie = "airoute_llm_session"

// maxSessionIDLen bounds client-supplied session identifiers. Session IDs are
// used verbatim as Redis key components and persisted into usage_logs, so an
// unbounded header value would bloat both stores.
const maxSessionIDLen = 128

// sanitizeSessionID trims and bounds a candidate session identifier.
func sanitizeSessionID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Drop control characters; keep the value otherwise opaque to the client.
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if len(s) > maxSessionIDLen {
		s = s[:maxSessionIDLen]
	}
	return s
}

// extractSessionID retrieves explicit LLM session affinity key from HTTP headers, cookies, query, or payload user.
func extractSessionID(c *gin.Context, userField string) string {
	if s := sanitizeSessionID(c.GetHeader("X-Session-ID")); s != "" {
		return s
	}
	if s := sanitizeSessionID(c.GetHeader("X-Conversation-ID")); s != "" {
		return s
	}
	if s := sanitizeSessionID(c.GetHeader("Session-Id")); s != "" {
		return s
	}
	if s := sanitizeSessionID(c.GetHeader("Conversation-Id")); s != "" {
		return s
	}
	if cookie, err := c.Cookie(llmSessionCookie); err == nil && cookie != "" {
		return sanitizeSessionID(cookie)
	}
	if cookie, err := c.Cookie("session_id"); err == nil && cookie != "" {
		return sanitizeSessionID(cookie)
	}
	if s := sanitizeSessionID(c.Query("session_id")); s != "" {
		return s
	}
	if userField != "" {
		return sanitizeSessionID(userField)
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

	// Truncate on rune boundaries so the hash input stays valid UTF-8.
	firstUserMsg = truncateRunes(firstUserMsg, 256)
	sysMsg = truncateRunes(sysMsg, 128)

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

// truncateRunes returns s truncated to at most n runes.
func truncateRunes(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

// resolveSessionID resolves either an explicit session ID or derives a zero-touch context fingerprint.
// It also writes sticky session headers and cookies onto the HTTP response.
func resolveSessionID(c *gin.Context, modelName string, messages []model.ChatMessage, userField string) string {
	sID := extractSessionID(c, userField)
	if sID == "" {
		sID = deriveContextFingerprint(c, modelName, messages)
	}
	if sID == "" {
		sID = fmt.Sprintf("sess_%d_%x", time.Now().Unix(), time.Now().UnixMicro()%1000000)
	}
	if sID != "" {
		c.Header("X-Airoute-Session-ID", sID)
		// Set cookie for browser-based clients (NextChat, OpenWebUI, LibreChat, Web App).
		// HttpOnly: the value is an affinity key, scripts have no reason to read it.
		c.SetCookie(llmSessionCookie, sID, 1800, "/", "", false, true)
	}
	return sID
}

// resolveGenericSessionID extracts sticky session identifiers or derives a unique fallback session.
func resolveGenericSessionID(c *gin.Context, prefix string) string {
	sID := sanitizeSessionID(c.GetHeader("X-Session-ID"))
	if sID == "" {
		sID = sanitizeSessionID(c.GetHeader("X-Airoute-Session-ID"))
	}
	if sID == "" {
		if cookie, err := c.Cookie(llmSessionCookie); err == nil && cookie != "" {
			sID = sanitizeSessionID(cookie)
		}
	}
	if sID == "" {
		if prefix == "" {
			prefix = "gen"
		}
		sID = fmt.Sprintf("sess_%s_%d_%x", prefix, time.Now().Unix(), time.Now().UnixMicro()%1000000)
	}
	c.Header("X-Airoute-Session-ID", sID)
	return sID
}
