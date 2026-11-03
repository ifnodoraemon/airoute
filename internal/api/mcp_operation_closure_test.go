package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestMCPConfiguredCommandKeepsQuotedArgumentsAndEnvironment(t *testing.T) {
	helper := writeMCPStdioHelper(t, false)
	wrapper := filepath.Join(t.TempDir(), "wrapper with spaces.sh")
	script := "#!/bin/sh\n[ \"$MCP_CLOSURE_VALUE\" = \"configured\" ] || exit 19\n[ \"$1\" = \"argument with spaces\" ] || exit 20\nexec " + strconv.Quote(helper) + "\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := newMCPClientTestRepo(t)
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{ID: "quoted-server", Name: "Quoted server", Enabled: true,
		Transport: "stdio", Endpoint: strconv.Quote(wrapper) + ` "argument with spaces"`,
		EnvVars: `{"MCP_CLOSURE_VALUE":"configured"}`, Tools: []string{"quoted_echo"}}); err != nil {
		t.Fatal(err)
	}
	h := &MCPHandler{repo: repo}
	text, isErr := h.executeDynamicMCPTool(context.Background(), "quoted_echo", nil)
	if isErr || text != "stdio-ok" {
		t.Fatalf("registered tool could not execute: %s isError=%v", text, isErr)
	}
}

func TestMCPNativeSwitchAndSkillDisableApplyToCalls(t *testing.T) {
	repo := newMCPClientTestRepo(t)
	if err := repo.SetMCPServerEnabled("airoute-gateway", false); err != nil {
		t.Fatal(err)
	}
	h := &MCPHandler{repo: repo}
	if _, isErr := h.executeTool(context.Background(), "airoute_data_redact", map[string]interface{}{"text": "hello"}); !isErr {
		t.Fatal("disabled native server still executed a tool")
	}
	listed := h.ProcessRPC(context.Background(), &mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"})
	body, _ := json.Marshal(listed)
	if strings.Contains(string(body), `"name":"airoute_data_redact"`) {
		t.Fatal("disabled native tool is still advertised")
	}
	if err := repo.SetMCPServerEnabled("airoute-gateway", true); err != nil {
		t.Fatal(err)
	}
	if _, isErr := h.executeTool(context.Background(), "airoute_data_redact", map[string]interface{}{"text": "hello"}); isErr {
		t.Fatal("re-enabled native server did not recover")
	}
	if err := repo.SetSkillEnabled("git-workflow", false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"airoute_inspect_skill", "airoute_get_skill_manifest"} {
		_, isErr := h.executeTool(context.Background(), name, map[string]interface{}{"skill_id": "git-workflow"})
		if !isErr {
			t.Fatalf("disabled skill was still loaded by %s", name)
		}
	}
}
