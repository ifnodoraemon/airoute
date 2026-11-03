package api

var builtinMCPToolSet = func() map[string]bool {
	names := make(map[string]bool)
	for _, tool := range getBuiltinMCPTools() {
		names[tool["name"].(string)] = true
	}
	return names
}()

// The native server's per-server switch must govern both discovery and calls.
// External tools remain available when only the native server is disabled.
func (h *MCPHandler) builtinMCPToolEnabled(name string) bool {
	if h.repo == nil || !builtinMCPToolSet[name] {
		return true
	}
	return h.nativeMCPServerEnabled()
}

func (h *MCPHandler) nativeMCPServerEnabled() bool {
	if h.repo == nil {
		return true
	}
	server, err := h.repo.GetMCPServer("airoute-gateway")
	return err == nil && server.Enabled
}
