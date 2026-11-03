package api

import (
	"context"
	"fmt"
	"github.com/ifnodoraemon/airoute/internal/model"
	"os/exec"
	"strings"
)

// executeDynamicMCPTool dispatches execution to configured external MCP servers
// from storage through the real MCP client transports (streamable HTTP, legacy
// HTTP/SSE or stdio) in mcp_client*.go.
func (h *MCPHandler) executeDynamicMCPTool(ctx context.Context, name string, args map[string]interface{}) (string, bool) {
	if h.repo != nil {
		servers, _ := h.repo.ListMCPServers()
		for _, srv := range servers {
			if !srv.Enabled {
				continue
			}
			for _, t := range srv.Tools {
				if t == name {
					endpoint := strings.TrimSpace(srv.Endpoint)
					if endpoint == "" {
						return fmt.Sprintf("MCP server '%s' has no endpoint configured", srv.Name), true
					}

					// HTTP(S): run the full MCP lifecycle over streamable HTTP
					// with a legacy HTTP/SSE fallback.
					if mcpIsHTTPEndpoint(endpoint) {
						return forwardHTTPMCPToolCall(ctx, srv.Transport, endpoint, name, args)
					}

					// Real stdio CLI execution if installed
					if srv.Transport == "stdio" {
						parts, err := model.ParseMCPCommand(endpoint)
						if err != nil {
							return err.Error(), true
						}
						if len(parts) > 0 {
							if _, lookErr := exec.LookPath(parts[0]); lookErr == nil {
								return executeStdioMCPTool(ctx, parts, name, args, srv.EnvVars)
							}
							return fmt.Sprintf("MCP server '%s' requires local runtime '%s' which is not installed", srv.Name, parts[0]), true
						}
					}

					return fmt.Sprintf("MCP server '%s' uses unsupported transport '%s'", srv.Name, srv.Transport), true
				}
			}
		}
	}
	return fmt.Sprintf("Unknown tool '%s'", name), true
}
