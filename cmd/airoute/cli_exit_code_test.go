package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Every CLI command must report failure through its process exit code. Before
// this, an unreachable gateway or a rejected admin call printed an error and
// exited 0, so `airoute keys || alert` never fired. These tests drive the real
// handlers against httptest servers and assert the returned code.
func TestCLIExitCodes(t *testing.T) {
	unreachable := "http://127.0.0.1:1"

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
		case "/api/v1/public/status":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"ok","channels":[],"models":[]}}`))
		case "/mcp":
			_, _ = w.Write([]byte(`{"server":{"name":"airoute","mcp_enabled":true,"enabled_skills":3}}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-chat","owned_by":"gpustack"}]}`))
		case "/api/v1/admin/keys":
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"code":0,"data":{"key":"sk-airoute-test"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/admin/users":
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/admin/skills":
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/admin/skills/web_search/toggle":
			_, _ = w.Write([]byte(`{"code":0}`))
		case "/api/v1/admin/mcp/settings":
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"code":0}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"mcp_enabled":true,"enabled_skills_count":3,"active_tools_count":12,"sse_endpoint":"/mcp/sse"}}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}],"usage":{"total_tokens":3}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer healthy.Close()

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"error":"boom"}`))
	}))
	defer failing.Close()

	tests := []struct {
		name string
		run  func() int
		want int
	}{
		// Unreachable gateway: every command must fail.
		{"status/unreachable", func() int { return handleStatus(unreachable, "") }, cliExitFail},
		{"models/unreachable", func() int { return handleModels(unreachable, "") }, cliExitFail},
		{"keys/unreachable", func() int { return handleKeys(unreachable, "t", nil) }, cliExitFail},
		{"users/unreachable", func() int { return handleUsers(unreachable, "t", nil) }, cliExitFail},
		{"skills/unreachable", func() int { return handleSkills(unreachable, "t", nil) }, cliExitFail},
		{"mcp/unreachable", func() int { return handleMCP(unreachable, "t", nil) }, cliExitFail},
		{"chat/unreachable", func() int { return handleChat(unreachable, "t", []string{"hi"}) }, cliExitFail},

		// Rejected calls (HTTP 500) must fail.
		{"status/http500", func() int { return handleStatus(failing.URL, "") }, cliExitFail},
		{"models/http500", func() int { return handleModels(failing.URL, "") }, cliExitFail},
		{"keys/http500", func() int { return handleKeys(failing.URL, "t", []string{"list"}) }, cliExitFail},
		{"keys-create/http500", func() int { return handleKeys(failing.URL, "t", []string{"create", "tenant"}) }, cliExitFail},
		{"users/http500", func() int { return handleUsers(failing.URL, "t", nil) }, cliExitFail},
		{"skills/http500", func() int { return handleSkills(failing.URL, "t", nil) }, cliExitFail},
		{"skills-toggle/http500", func() int { return handleSkills(failing.URL, "t", []string{"enable", "web_search"}) }, cliExitFail},
		{"mcp/http500", func() int { return handleMCP(failing.URL, "t", nil) }, cliExitFail},
		{"mcp-enable/http500", func() int { return handleMCP(failing.URL, "t", []string{"enable"}) }, cliExitFail},
		{"chat/http500", func() int { return handleChat(failing.URL, "t", []string{"hi"}) }, cliExitFail},

		// Unknown subcommands are errors too.
		{"keys/unknown-subcommand", func() int { return handleKeys(healthy.URL, "t", []string{"bogus"}) }, cliExitFail},
		{"users/unknown-subcommand", func() int { return handleUsers(healthy.URL, "t", []string{"bogus"}) }, cliExitFail},
		{"skills/unknown-subcommand", func() int { return handleSkills(healthy.URL, "t", []string{"bogus"}) }, cliExitFail},
		{"mcp/unknown-subcommand", func() int { return handleMCP(healthy.URL, "t", []string{"bogus"}) }, cliExitFail},
		{"users/missing-args", func() int { return handleUsers(healthy.URL, "t", []string{"create", "only-user"}) }, cliExitFail},

		// Successful calls must still exit 0 (the exit-code fix must not turn
		// ordinary output into failures).
		{"status/ok", func() int { return handleStatus(healthy.URL, "") }, cliExitOK},
		{"models/ok", func() int { return handleModels(healthy.URL, "") }, cliExitOK},
		{"keys/ok", func() int { return handleKeys(healthy.URL, "t", []string{"list"}) }, cliExitOK},
		{"keys-create/ok", func() int { return handleKeys(healthy.URL, "t", []string{"create", "tenant"}) }, cliExitOK},
		{"users/ok", func() int { return handleUsers(healthy.URL, "t", nil) }, cliExitOK},
		{"skills/ok", func() int { return handleSkills(healthy.URL, "t", nil) }, cliExitOK},
		{"skills-toggle/ok", func() int { return handleSkills(healthy.URL, "t", []string{"enable", "web_search"}) }, cliExitOK},
		{"mcp/ok", func() int { return handleMCP(healthy.URL, "t", nil) }, cliExitOK},
		{"mcp-enable/ok", func() int { return handleMCP(healthy.URL, "t", []string{"enable"}) }, cliExitOK},
		{"chat/ok", func() int { return handleChat(healthy.URL, "t", []string{"hi"}) }, cliExitOK},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.run(); got != tc.want {
				t.Fatalf("exit code = %d, want %d", got, tc.want)
			}
		})
	}
}

// `airoute chat` used to fall back to a hardcoded API key literal that exists on
// no deployment, so it always returned 401 with a confusing error. It must now
// refuse to run without AIROUTE_TOKEN and say how to get one.
func TestChatRequiresToken(t *testing.T) {
	t.Setenv("AIROUTE_TOKEN", "")
	if got := handleChat("http://127.0.0.1:1", "", []string{"hi"}); got != cliExitFail {
		t.Fatalf("chat without a token exited %d, want %d", got, cliExitFail)
	}
}
