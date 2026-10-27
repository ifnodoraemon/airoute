package api

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// writeMCPStdioHelper writes a tiny newline-delimited JSON-RPC MCP server: it
// logs a non-JSON line, answers initialize and tools/call, and exits on EOF.
func writeMCPStdioHelper(t *testing.T, toolIsError bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stdio helper script requires a POSIX shell")
	}
	script := `#!/bin/sh
printf 'helper starting up\n'
while IFS= read -r line; do
  id=$(printf '%s' "$line" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"helper","version":"1.0"}}}\n' "$id"
      ;;
    *'"method":"tools/call"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"stdio-ok"}],"isError":__IS_ERROR__}}\n' "$id"
      ;;
  esac
done
`
	script = strings.ReplaceAll(script, "__IS_ERROR__", map[bool]string{true: "true", false: "false"}[toolIsError])
	path := filepath.Join(t.TempDir(), "mcp-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stdio helper: %v", err)
	}
	return path
}

// #12: stdio must run the initialize handshake before tools/call, not write a
// bare tools/call and close stdin.
func TestExecuteStdioMCPToolHandshake(t *testing.T) {
	helper := writeMCPStdioHelper(t, false)

	text, isErr := executeStdioMCPTool(context.Background(), []string{helper}, "echo", map[string]interface{}{"q": "hi"})
	if isErr {
		t.Fatalf("stdio tool call failed: %s", text)
	}
	if text != "stdio-ok" {
		t.Fatalf("text = %q, want stdio-ok", text)
	}
}

func TestExecuteStdioMCPToolPropagatesIsError(t *testing.T) {
	helper := writeMCPStdioHelper(t, true)

	text, isErr := executeStdioMCPTool(context.Background(), []string{helper}, "echo", nil)
	if !isErr {
		t.Fatalf("isError was not propagated: text=%q", text)
	}
	if text != "stdio-ok" {
		t.Fatalf("text = %q, want stdio-ok", text)
	}
}

func TestExecuteStdioMCPToolMissingCommand(t *testing.T) {
	text, isErr := executeStdioMCPTool(context.Background(), []string{"/nonexistent/airoute-mcp-helper"}, "echo", nil)
	if !isErr {
		t.Fatalf("expected an error for a missing command, got %q", text)
	}
}

func TestExecuteStdioMCPToolHonorsContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio helper script requires a POSIX shell")
	}
	script := "#!/bin/sh\nexec sleep 5\n"
	path := filepath.Join(t.TempDir(), "slow-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write slow helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, isErr := executeStdioMCPTool(ctx, []string{path}, "echo", nil)
	if !isErr {
		t.Fatal("expected a timeout error from the stdio server")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("stdio call ignored the context deadline (took %v)", elapsed)
	}
}

// #12: the dynamic tool dispatcher must pick the stdio transport from storage.
func TestExecuteDynamicMCPToolStdio(t *testing.T) {
	helper := writeMCPStdioHelper(t, false)

	repo := newMCPClientTestRepo(t)
	if err := repo.SaveMCPServer(&storage.MCPServerRecord{
		ID: "ext-stdio", Name: "Ext stdio", Transport: "stdio", Endpoint: helper,
		Status: "online", Enabled: true, Tools: []string{"stdio_echo"},
	}); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	handler := &MCPHandler{repo: repo}
	text, isErr := handler.executeDynamicMCPTool(context.Background(), "stdio_echo", nil)
	if isErr {
		t.Fatalf("executeDynamicMCPTool failed: %s", text)
	}
	if text != "stdio-ok" {
		t.Fatalf("text = %q, want stdio-ok", text)
	}
}
