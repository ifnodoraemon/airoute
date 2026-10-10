package api

import "encoding/json"

// MCP JSON-RPC 2.0 protocol models.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
	Meta    interface{} `json:"_meta,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpTextContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// newMCPResultResponse creates a successful JSON-RPC 2.0 response.
func newMCPResultResponse(id interface{}, result interface{}, meta ...interface{}) mcpResponse {
	resp := mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	if len(meta) > 0 && meta[0] != nil {
		resp.Meta = meta[0]
	}
	return resp
}

// newMCPErrorResponse creates an error JSON-RPC 2.0 response.
func newMCPErrorResponse(id interface{}, code int, message string) mcpResponse {
	return mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &mcpError{
			Code:    code,
			Message: message,
		},
	}
}

// newMCPToolResponse creates a standard tool execution result response.
func newMCPToolResponse(id interface{}, text string, isError bool) mcpResponse {
	return mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: mcpToolResult{
			Content: []mcpTextContent{
				{Type: "text", Text: text},
			},
			IsError: isError,
		},
	}
}
