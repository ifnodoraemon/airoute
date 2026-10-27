package api

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// A persistent MCP SSE channel must survive http.Server.WriteTimeout. WriteTimeout
// is an absolute deadline for the whole response, not an idle timeout, so without
// clearing it the channel is cut mid-stream (600s with the shipped Compose config,
// 180s with the Helm chart). This test uses a 150ms server write timeout and then
// proves the connection still accepts a message after sleeping well past it.
func TestMCPSSEOutlivesServerWriteTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewMCPHandler(nil, nil) // no repository => open development mode
	engine.GET("/mcp/sse", handler.HandleMCPSSE)

	srv := httptest.NewUnstartedServer(engine)
	srv.Config.WriteTimeout = 150 * time.Millisecond
	srv.Start()
	defer srv.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(srv.URL + "/mcp/sse")
	if err != nil {
		t.Fatalf("connect to MCP SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	endpoint, err := readSSEEvent(reader, "endpoint")
	if err != nil {
		t.Fatalf("read endpoint event: %v", err)
	}
	const marker = "sessionId="
	idx := strings.LastIndex(endpoint, marker)
	if idx < 0 {
		t.Fatalf("endpoint event %q does not contain a session id", endpoint)
	}
	sessionID := strings.TrimSpace(endpoint[idx+len(marker):])

	// Cross the server write timeout while the stream is open. With an uncleared
	// absolute deadline every later write fails and nothing reaches the client.
	time.Sleep(400 * time.Millisecond)

	sessAny, ok := handler.sessions.Load(sessionID)
	if !ok {
		t.Fatalf("session %q not registered", sessionID)
	}
	sess, ok := sessAny.(*mcpSession)
	if !ok {
		t.Fatalf("unexpected session type %T", sessAny)
	}
	if !sess.Send([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)) {
		t.Fatal("session rejected the message")
	}

	msg, err := readSSEEvent(reader, "message")
	if err != nil {
		t.Fatalf("read message event after server WriteTimeout: %v", err)
	}
	if !strings.Contains(msg, `"ok":true`) {
		t.Fatalf("unexpected message payload %q", msg)
	}
}

// readSSEEvent reads until it finds a data line belonging to wantEvent.
func readSSEEvent(r *bufio.Reader, wantEvent string) (string, error) {
	event := ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("waiting for event %q: %w", wantEvent, err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if event == wantEvent {
				return strings.TrimPrefix(line, "data: "), nil
			}
		}
	}
}
