package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ---------------------------------------------------------------------------
// Streamable HTTP test server: POST initialize -> capture Mcp-Session-Id ->
// notifications/initialized -> tools/call, replying with JSON or SSE.
// ---------------------------------------------------------------------------

type streamableTestServer struct {
	mu               sync.Mutex
	sseReply         bool
	toolText         string
	toolIsError      bool
	sawInitialize    bool
	sawInitProtocol  string
	sawInitialized   bool
	sawSessionOnCall bool
	sawToolName      string
}

func (s *streamableTestServer) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
				ClientInfo      struct {
					Name string `json:"name"`
				} `json:"clientInfo"`
			}
			_ = json.Unmarshal(req.Params, &params)
			s.mu.Lock()
			s.sawInitialize = true
			s.sawInitProtocol = params.ProtocolVersion
			s.mu.Unlock()
			w.Header().Set("Mcp-Session-Id", "sess-test-1")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"test","version":"1.0"}}}`,
				string(req.ID))
		case "notifications/initialized":
			s.mu.Lock()
			s.sawInitialized = r.Header.Get("Mcp-Session-Id") == "sess-test-1"
			s.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			var params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			s.mu.Lock()
			s.sawSessionOnCall = r.Header.Get("Mcp-Session-Id") == "sess-test-1"
			s.sawToolName = params.Name
			text, isErr, sse := s.toolText, s.toolIsError, s.sseReply
			s.mu.Unlock()

			result, _ := json.Marshal(map[string]interface{}{
				"content": []map[string]interface{}{{"type": "text", "text": text}},
				"isError": isErr,
			})
			reply := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, string(req.ID), string(result))
			if !sse {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(reply))
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			// Interleave a server notification before the actual reply: the
			// client must not mistake it for the response to its request id.
			fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\",\"params\":{\"level\":\"info\"}}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", reply)
			if flusher != nil {
				flusher.Flush()
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

func TestCallStreamableHTTPMCPToolJSON(t *testing.T) {
	server := &streamableTestServer{toolText: "streamable-ok"}
	srv := httptest.NewServer(server.handler(t))
	t.Cleanup(srv.Close)

	text, isErr, err := callStreamableHTTPMCPTool(context.Background(), srv.URL, "echo", map[string]interface{}{"q": "hi"})
	if err != nil {
		t.Fatalf("callStreamableHTTPMCPTool: %v", err)
	}
	if isErr || text != "streamable-ok" {
		t.Fatalf("result text=%q isErr=%v", text, isErr)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.sawInitialize || server.sawInitProtocol != mcpClientProtocolVersion {
		t.Fatalf("initialize not sent with protocol %q", server.sawInitProtocol)
	}
	if !server.sawInitialized {
		t.Fatal("notifications/initialized was not sent with the session id")
	}
	if !server.sawSessionOnCall {
		t.Fatal("tools/call did not echo the Mcp-Session-Id captured from initialize")
	}
	if server.sawToolName != "echo" {
		t.Fatalf("tools/call name = %q", server.sawToolName)
	}
}

func TestCallStreamableHTTPMCPToolSSEReply(t *testing.T) {
	server := &streamableTestServer{toolText: "sse-ok", sseReply: true}
	srv := httptest.NewServer(server.handler(t))
	t.Cleanup(srv.Close)

	text, isErr, err := callStreamableHTTPMCPTool(context.Background(), srv.URL, "echo", nil)
	if err != nil {
		t.Fatalf("callStreamableHTTPMCPTool: %v", err)
	}
	if isErr || text != "sse-ok" {
		t.Fatalf("result text=%q isErr=%v", text, isErr)
	}
}

func TestCallStreamableHTTPMCPToolPropagatesIsError(t *testing.T) {
	server := &streamableTestServer{toolText: "tool blew up", toolIsError: true}
	srv := httptest.NewServer(server.handler(t))
	t.Cleanup(srv.Close)

	text, isErr, err := callStreamableHTTPMCPTool(context.Background(), srv.URL, "echo", nil)
	if err != nil {
		t.Fatalf("callStreamableHTTPMCPTool: %v", err)
	}
	if !isErr || text != "tool blew up" {
		t.Fatalf("isError was not propagated: text=%q isErr=%v", text, isErr)
	}
}

func TestCallStreamableHTTPMCPToolUnreachable(t *testing.T) {
	_, _, err := callStreamableHTTPMCPTool(context.Background(), "http://127.0.0.1:1/mcp", "echo", nil)
	if err == nil {
		t.Fatal("expected an error for an unreachable server")
	}
}

func TestCallStreamableHTTPMCPToolHonorsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold the reply past the client deadline; bound the handler itself so
		// httptest.Server.Close does not wait on the aborted connection.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := callStreamableHTTPMCPTool(ctx, srv.URL, "echo", nil)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("call ignored the 150ms context deadline (took %v)", elapsed)
	}
}

func TestCallStreamableHTTPMCPToolResponseCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"`)
		_, _ = io.WriteString(w, strings.Repeat("a", int(mcpClientMaxResponseBytes)+1024))
		_, _ = io.WriteString(w, `"}`)
	}))
	t.Cleanup(srv.Close)

	_, _, err := callStreamableHTTPMCPTool(context.Background(), srv.URL, "echo", nil)
	if err == nil {
		t.Fatal("expected the 8 MiB response cap to reject the oversized reply")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Legacy HTTP/SSE test server: GET event stream advertising an `endpoint`,
// replies delivered as `message` events after the client POSTs JSON-RPC.
// ---------------------------------------------------------------------------

type legacyTestServer struct {
	responses      chan string
	mu             sync.Mutex
	sawInitialize  bool
	sawInitialized bool
	sawToolCall    bool
	sessionQueryOK bool
	toolText       string
}

func newLegacyTestServer(t *testing.T, toolText string) (*legacyTestServer, *httptest.Server) {
	t.Helper()
	server := &legacyTestServer{responses: make(chan string, 8), toolText: toolText}
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", server.streamHandler)
	mux.HandleFunc("/messages", server.messageHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return server, srv
}

func (s *legacyTestServer) streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	fmt.Fprint(w, "event: endpoint\ndata: /messages?sessionId=sess-legacy\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	for {
		select {
		case message := <-s.responses:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", message)
			if flusher != nil {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *legacyTestServer) messageHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &req)
	queryOK := r.URL.Query().Get("sessionId") == "sess-legacy"

	switch req.Method {
	case "initialize":
		s.mu.Lock()
		s.sawInitialize = true
		s.sessionQueryOK = queryOK
		s.mu.Unlock()
		s.responses <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"legacy","version":"1.0"}}}`, string(req.ID))
	case "notifications/initialized":
		s.mu.Lock()
		s.sawInitialized = true
		s.mu.Unlock()
	case "tools/call":
		s.mu.Lock()
		s.sawToolCall = true
		text := s.toolText
		s.mu.Unlock()
		s.responses <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":%q}],"isError":false}}`, string(req.ID), text)
	}
	w.WriteHeader(http.StatusAccepted)
}

func TestCallLegacySSEMCPTool(t *testing.T) {
	server, srv := newLegacyTestServer(t, "legacy-ok")

	text, isErr, err := callLegacySSEMCPTool(context.Background(), srv.URL+"/sse", "echo", map[string]interface{}{"q": "hi"})
	if err != nil {
		t.Fatalf("callLegacySSEMCPTool: %v", err)
	}
	if isErr || text != "legacy-ok" {
		t.Fatalf("result text=%q isErr=%v", text, isErr)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.sawInitialize || !server.sessionQueryOK {
		t.Fatal("initialize was not POSTed to the advertised endpoint URL")
	}
	if !server.sawInitialized {
		t.Fatal("notifications/initialized was not POSTed")
	}
	if !server.sawToolCall {
		t.Fatal("tools/call was not POSTed")
	}
}

// A server that only speaks legacy SSE answers 405 to the streamable POST; the
// client must fall back and complete the legacy lifecycle.
func TestForwardJSONRPCToolCallFallsBackToLegacySSE(t *testing.T) {
	server, _ := newLegacyTestServer(t, "fallback-ok")

	mux := http.NewServeMux()
	mux.HandleFunc("/messages", server.messageHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		server.streamHandler(w, r)
	})
	legacyServer := httptest.NewServer(mux)
	t.Cleanup(legacyServer.Close)

	text, isErr := forwardJSONRPCToolCall(context.Background(), legacyServer.URL, "echo", nil)
	if isErr {
		t.Fatalf("fallback failed: %s", text)
	}
	if text != "fallback-ok" {
		t.Fatalf("fallback text = %q, want fallback-ok", text)
	}
}

func TestLegacySSEMCPToolUnreachable(t *testing.T) {
	_, _, err := callLegacySSEMCPTool(context.Background(), "http://127.0.0.1:1/sse", "echo", nil)
	if err == nil {
		t.Fatal("expected an error for an unreachable SSE server")
	}
	if !isMCPFallbackError(err) {
		t.Fatalf("connection failure should be fallback-safe, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Wiring: executeDynamicMCPTool must route to the transport configured in
// storage, and the MCP handler must surface the resulting text.
// ---------------------------------------------------------------------------

func newMCPClientTestRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return storage.NewRepository(db)
}

func TestExecuteDynamicMCPToolStreamableHTTP(t *testing.T) {
	server := &streamableTestServer{toolText: "dynamic-http-ok"}
	srv := httptest.NewServer(server.handler(t))
	t.Cleanup(srv.Close)

	repo := newMCPClientTestRepo(t)
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{
		ID: "ext-http", Name: "Ext HTTP", Transport: "http", Endpoint: srv.URL,
		Status: "online", Enabled: true, Tools: []string{"ext_echo"},
	}); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	handler := &MCPHandler{repo: repo}
	text, isErr := handler.executeDynamicMCPTool(context.Background(), "ext_echo", map[string]interface{}{"q": "x"})
	if isErr {
		t.Fatalf("executeDynamicMCPTool failed: %s", text)
	}
	if text != "dynamic-http-ok" {
		t.Fatalf("text = %q, want dynamic-http-ok", text)
	}
}

func TestExecuteDynamicMCPToolLegacySSE(t *testing.T) {
	_, srv := newLegacyTestServer(t, "dynamic-sse-ok")

	repo := newMCPClientTestRepo(t)
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{
		ID: "ext-sse", Name: "Ext SSE", Transport: "sse", Endpoint: srv.URL + "/sse",
		Status: "online", Enabled: true, Tools: []string{"legacy_echo"},
	}); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	handler := &MCPHandler{repo: repo}
	text, isErr := handler.executeDynamicMCPTool(context.Background(), "legacy_echo", nil)
	if isErr {
		t.Fatalf("executeDynamicMCPTool failed: %s", text)
	}
	if text != "dynamic-sse-ok" {
		t.Fatalf("text = %q, want dynamic-sse-ok", text)
	}
}
