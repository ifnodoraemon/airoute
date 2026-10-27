package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newMCPTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

// #11: local-gateway detection must be exact. A substring match on "/mcp"
// classified every external https://host/mcp endpoint as the local gateway.
func TestIsLocalGatewayMCPServerExactDetection(t *testing.T) {
	const base = "http://gateway.test"
	cases := []struct {
		name     string
		id       string
		endpoint string
		want     bool
	}{
		{"builtin gateway id with any endpoint", "airoute-gateway", "https://external.example/mcp", true},
		{"relative endpoint", "custom-1", "/mcp/sse", true},
		{"gateway public sse url", "custom-2", "http://gateway.test/mcp/sse", true},
		{"gateway public url with trailing slash", "custom-3", "http://gateway.test/mcp/sse/", true},
		{"gateway root mcp url", "custom-4", "http://gateway.test/mcp", true},
		{"external mcp path", "custom-5", "https://external.example/mcp", false},
		{"external sse endpoint", "custom-6", "https://mcp.modelscope.cn/servers/search/sse", false},
		{"external with /mcp prefix path", "custom-7", "https://external.example/mcp/sse", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLocalGatewayMCPServer(tc.id, tc.endpoint, base); got != tc.want {
				t.Fatalf("isLocalGatewayMCPServer(%q, %q) = %v, want %v", tc.id, tc.endpoint, got, tc.want)
			}
		})
	}
}

// #11: listing must keep an external endpoint byte-for-byte and still rewrite
// only the gateway's own record to the current public URL.
func TestListMCPServersKeepsExternalEndpoint(t *testing.T) {
	t.Setenv("PUBLIC_URL", "http://gateway.test")
	handler, repo := newMCPTestHandler(t)

	externalEndpoint := "https://external.example/mcp"
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{
		ID: "ext-mcp", Name: "External MCP", Transport: "http", Endpoint: externalEndpoint,
		Status: "online", Enabled: true, Tools: []string{"ext_tool"},
	}); err != nil {
		t.Fatalf("seed external server: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://gateway.test/api/v1/admin/mcp/servers", nil)
	handler.ListMCPServers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data []storage.MCPServerRecord `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	var sawExternal, sawLocal bool
	for _, s := range body.Data {
		switch s.ID {
		case "ext-mcp":
			sawExternal = true
			if s.Endpoint != externalEndpoint {
				t.Fatalf("external endpoint rewritten to %q, want %q", s.Endpoint, externalEndpoint)
			}
		case "airoute-gateway":
			sawLocal = true
			if s.Endpoint != "http://gateway.test/mcp/sse" {
				t.Fatalf("local gateway endpoint = %q, want the public URL", s.Endpoint)
			}
		}
	}
	if !sawExternal || !sawLocal {
		t.Fatalf("missing servers in list: external=%v local=%v", sawExternal, sawLocal)
	}
}

// #11: an unreachable external host must really be dialled and reported
// offline, instead of the old synthetic "online, 1ms".
func TestProbeMCPServerUnreachableExternalHostIsOffline(t *testing.T) {
	t.Setenv("PUBLIC_URL", "http://gateway.test")
	handler, repo := newMCPTestHandler(t)

	// Port 1 on loopback is closed, so the dial fails immediately.
	const endpoint = "http://127.0.0.1:1/mcp"
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{
		ID: "ext-dead", Name: "Dead MCP", Transport: "sse", Endpoint: endpoint,
		Status: "online", Enabled: true, Tools: []string{"dead_tool"},
	}); err != nil {
		t.Fatalf("seed dead server: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "http://gateway.test/api/v1/admin/mcp/servers/ext-dead/probe", nil)
	c.Params = gin.Params{{Key: "id", Value: "ext-dead"}}
	handler.ProbeMCPServer(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Status     string `json:"status"`
			LatencyMS  int64  `json:"latency_ms"`
			Endpoint   string `json:"endpoint"`
			ProbeError string `json:"probe_error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode probe response: %v", err)
	}
	if body.Data.Status == "online" {
		t.Fatalf("unreachable external host reported online with latency %dms", body.Data.LatencyMS)
	}
	if body.Data.ProbeError == "" {
		t.Fatal("expected a probe_error for an unreachable host")
	}
	if body.Data.Endpoint != endpoint {
		t.Fatalf("probe endpoint = %q, want the external URL unchanged", body.Data.Endpoint)
	}
	if stored, err := repo.GetMCPServer("ext-dead"); err != nil || stored.Status != "offline" {
		t.Fatalf("stored status = %+v err=%v, want offline", stored, err)
	}
}

// #11: a reachable external /mcp endpoint is genuinely dialled, not faked.
func TestProbeMCPServerDialsExternalEndpoint(t *testing.T) {
	t.Setenv("PUBLIC_URL", "http://gateway.test")
	handler, repo := newMCPTestHandler(t)

	var dialled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dialled = true
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			http.Error(w, "expected SSE accept header", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	endpoint := srv.URL + "/mcp"
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{
		ID: "ext-live", Name: "Live MCP", Transport: "sse", Endpoint: endpoint,
		Status: "offline", Enabled: true, Tools: []string{"live_tool"},
	}); err != nil {
		t.Fatalf("seed live server: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "http://gateway.test/api/v1/admin/mcp/servers/ext-live/probe", nil)
	c.Params = gin.Params{{Key: "id", Value: "ext-live"}}
	handler.ProbeMCPServer(c)

	if !dialled {
		t.Fatal("probe never dialled the external endpoint")
	}
	var body struct {
		Data struct {
			Status   string `json:"status"`
			Endpoint string `json:"endpoint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode probe response: %v", err)
	}
	if body.Data.Status != "online" {
		t.Fatalf("status=%q, want online (body=%s)", body.Data.Status, w.Body.String())
	}
	if body.Data.Endpoint != endpoint {
		t.Fatalf("probe endpoint = %q, want %q", body.Data.Endpoint, endpoint)
	}
}
