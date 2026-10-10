package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	b := make([]byte, 8)
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

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSE streaming unsupported"})
		return
	}

	callerKey := extractMCPKey(c)
	sessionID := genSessionID()
	sess := &mcpSession{ch: make(chan []byte, 32), apiKey: callerKey}
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
