package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// mcpSession wraps a channel with mutual exclusion and state tracking to prevent panics on closed channels.
type mcpSession struct {
	mu     sync.Mutex
	ch     chan []byte
	apiKey string
	closed bool
}

func (s *mcpSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

func (s *mcpSession) Send(msg []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- msg:
		return true
	default:
		return false
	}
}

func extractMCPKey(c *gin.Context) string {
	authH := c.GetHeader("Authorization")
	if strings.HasPrefix(authH, "Bearer ") {
		return strings.TrimPrefix(authH, "Bearer ")
	}
	if k := c.GetHeader("X-API-Key"); k != "" {
		return k
	}
	if k := c.GetHeader("x-api-key"); k != "" {
		return k
	}
	if k := c.Query("apiKey"); k != "" {
		return k
	}
	if k := c.Query("token"); k != "" {
		return k
	}
	if k := c.Query("key"); k != "" {
		return k
	}
	return ""
}

func genSessionID() string {
	// 128 bits: when no API key is supplied on the POST, the sessionId itself is
	// the only capability protecting the SSE channel, so it must not be
	// brute-forceable (A08 raised the previous 64-bit value). The POST binding
	// below is the primary control; entropy is defence in depth for log leaks.
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "mcp-" + hex.EncodeToString(b)
}

// HandleMCPSSE handles GET /mcp/sse (initiates standard MCP SSE channel).
func (h *MCPHandler) HandleMCPSSE(c *gin.Context) {
	if h.repo != nil && h.repo.GetSetting("mcp_enabled", "true") == "false" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP 服务在 Airoute 网关中已按需关闭。如需使用，请在控制台开启 MCP 服务。",
		})
		return
	}

	// Authenticate BEFORE the response is switched to text/event-stream. A11:
	// gin's renderer only sets Content-Type when it is still empty, so rejecting
	// the request after the SSE headers were installed served a JSON error body
	// labelled text/event-stream — strict MCP clients then tried to parse the
	// rejection as an event stream and reported an unreadable handshake failure.
	callerKey := storage.HashAPIKey(extractMCPKey(c))
	reqCtx := c.Request.Context()
	caller, accessErr := h.resolveCallerDigest(callerKey)
	if accessErr != nil {
		// Preserve the historical wording for a credential failure; data-plane
		// rejections (locked account, exhausted budget/wallet) speak for themselves.
		message := accessErr.Message
		if accessErr.StatusCode == http.StatusUnauthorized {
			message = "Authentication required for MCP SSE connection: " + strings.TrimPrefix(message, "Unauthorized: ")
		}
		c.JSON(accessErr.StatusCode, gin.H{"error": message})
		return
	}
	if !h.checkCallerRateLimits(reqCtx, c, caller) {
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSE streaming unsupported"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	// An MCP SSE channel is a persistent connection: it lives until the client
	// goes away, not until the current request is done. http.Server.WriteTimeout
	// is an absolute deadline for the whole response (not an idle timeout), so
	// without clearing it here the channel is cut mid-stream at 600s with the
	// shipped Compose config and 180s with the Helm chart. This mirrors what
	// InitSSEStream does for the data-plane inference streams.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})

	sessionID := genSessionID()
	sess := &mcpSession{ch: make(chan []byte, 32), apiKey: caller.Key}
	h.sessions.Store(sessionID, sess)
	defer func() {
		h.sessions.Delete(sessionID)
		sess.Close()
	}()

	// Send endpoint event as required by MCP SSE transport
	endpointURI := fmt.Sprintf("/mcp/messages?sessionId=%s", sessionID)
	fmt.Fprintf(c.Writer, "event: endpoint\ndata: %s\n\n", endpointURI)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case msg, open := <-sess.ch:
			if !open {
				return
			}
			fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", string(msg))
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(c.Writer, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
