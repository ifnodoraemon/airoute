package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HTTP transports for the outbound MCP client.
//
// Streamable HTTP (MCP 2025-03-26): every message is a POST to the server's MCP
// endpoint, the server answers inline either with application/json or with a
// text/event-stream that carries the JSON-RPC reply; the session id advertised in
// the initialize response's Mcp-Session-Id header must be echoed on every later
// request, and an initialized notification completes the lifecycle.
//
// Legacy HTTP/SSE (MCP 2024-11-05): the client GETs the SSE endpoint, waits for
// the `endpoint` event that advertises the message URL, POSTs JSON-RPC there and
// reads the replies as `message` events on the still-open GET stream.

// mcpFallbackStatus reports HTTP statuses that mean "this endpoint does not speak
// the transport I tried", which justifies retrying with the other transport.
func mcpFallbackStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusNotAcceptable, http.StatusUnsupportedMediaType, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

// mcpNewHTTPRequest builds a JSON-RPC POST with the MCP headers.
func mcpNewHTTPRequest(ctx context.Context, endpoint string, payload []byte, sessionID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
		req.Header.Set("MCP-Protocol-Version", mcpClientProtocolVersion)
	}
	return req, nil
}

// callStreamableHTTPMCPTool runs the full streamable-HTTP lifecycle and returns
// the tool text and its isError flag.
func callStreamableHTTPMCPTool(ctx context.Context, endpoint, toolName string, args map[string]interface{}) (string, bool, error) {
	callCtx, cancel := mcpCallContext(ctx)
	defer cancel()

	// 1. initialize — the server may answer with the session id header.
	initID := mcpNextClientID()
	payload, err := mcpBuildRequest(initID, "initialize", mcpInitializePayload())
	if err != nil {
		return "", true, fmt.Errorf("failed to serialize MCP initialize request: %w", err)
	}
	initResp, sessionID, err := mcpStreamableRequest(callCtx, endpoint, "", payload, initID, true)
	if err != nil {
		return "", true, err
	}
	if initResp.Error != nil {
		return mcpRPCErrorText(initResp.Error), true, nil
	}
	if len(initResp.Result) == 0 {
		return "", true, fmt.Errorf("MCP server '%s' returned an empty initialize result", endpoint)
	}

	// 2. notifications/initialized — a notification, no reply expected.
	notifyPayload, err := mcpBuildRequest(0, "notifications/initialized", nil)
	if err != nil {
		return "", true, fmt.Errorf("failed to serialize MCP initialized notification: %w", err)
	}
	if err := mcpStreamableNotify(callCtx, endpoint, sessionID, notifyPayload); err != nil {
		return "", true, err
	}

	// 3. tools/call
	callID := mcpNextClientID()
	payload, err = mcpBuildRequest(callID, "tools/call", mcpToolsCallPayload(toolName, args))
	if err != nil {
		return "", true, fmt.Errorf("failed to serialize MCP tools/call request: %w", err)
	}
	resp, _, err := mcpStreamableRequest(callCtx, endpoint, sessionID, payload, callID, false)
	if err != nil {
		return "", true, err
	}
	if resp.Error != nil {
		return mcpRPCErrorText(resp.Error), true, nil
	}
	text, isErr := mcpParseToolResult(resp.Result)
	return text, isErr, nil
}

// mcpStreamableRequest performs one JSON-RPC request and returns its reply plus
// the (possibly new) session id. allowFallback is true only for messages sent
// before any tool work happened, so a failing tools/call is never replayed on
// another transport.
func mcpStreamableRequest(ctx context.Context, endpoint, sessionID string, payload []byte, id int64, allowFallback bool) (*mcpClientResponse, string, error) {
	req, err := mcpNewHTTPRequest(ctx, endpoint, payload, sessionID)
	if err != nil {
		return nil, sessionID, fmt.Errorf("failed to initialize HTTP request to '%s': %w", endpoint, err)
	}
	resp, err := mcpClientHTTPClient.Do(req)
	if err != nil {
		connErr := mcpTransportError(endpoint, err)
		if allowFallback {
			return nil, sessionID, mcpFallbackError("%v", connErr)
		}
		return nil, sessionID, connErr
	}
	defer resp.Body.Close()

	if newSession := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); newSession != "" {
		sessionID = newSession
	}

	if resp.StatusCode >= 400 {
		body, _ := mcpReadBody(resp.Body)
		statusErr := fmt.Errorf("upstream MCP server '%s' returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
		if allowFallback && mcpFallbackStatus(resp.StatusCode) {
			return nil, sessionID, mcpFallbackError("%v", statusErr)
		}
		return nil, sessionID, statusErr
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		reply, err := mcpReadStreamableSSEReply(ctx, resp.Body, endpoint, id)
		return reply, sessionID, err
	}

	body, err := mcpReadBody(resp.Body)
	if err != nil {
		return nil, sessionID, fmt.Errorf("failed reading response from upstream MCP server: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, sessionID, fmt.Errorf("MCP server '%s' accepted request id %d without a JSON-RPC response", endpoint, id)
	}
	responses, err := mcpDecodeRPCResponses(body)
	if err != nil {
		return nil, sessionID, fmt.Errorf("invalid JSON-RPC response from MCP server '%s': %w", endpoint, err)
	}
	if reply := mcpPickResponse(responses, id); reply != nil {
		return reply, sessionID, nil
	}
	return nil, sessionID, fmt.Errorf("MCP server '%s' did not return a response for request id %d", endpoint, id)
}

// mcpReadStreamableSSEReply reads a POST response that is an SSE stream and stops
// at the matching JSON-RPC reply. An `endpoint` event means the server is a
// legacy SSE server, so the caller may fall back.
func mcpReadStreamableSSEReply(ctx context.Context, body io.ReadCloser, endpoint string, id int64) (*mcpClientResponse, error) {
	stream := newMCPSSEStream(body)
	defer stream.close()
	for {
		event, err := stream.next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("MCP server '%s' closed the event stream without a response for request id %d", endpoint, id)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("MCP server '%s' timed out waiting for a response: %w", endpoint, err)
			}
			return nil, mcpTransportError(endpoint, err)
		}
		if event.Event == "endpoint" {
			return nil, mcpFallbackError("MCP server '%s' speaks legacy SSE, not streamable HTTP", endpoint)
		}
		if event.Event != "" && event.Event != "message" {
			continue
		}
		responses, err := mcpDecodeRPCResponses([]byte(event.Data))
		if err != nil {
			continue
		}
		if reply := mcpPickResponse(responses, id); reply != nil {
			return reply, nil
		}
	}
}

// mcpStreamableNotify posts a JSON-RPC notification and drains any response.
func mcpStreamableNotify(ctx context.Context, endpoint, sessionID string, payload []byte) error {
	req, err := mcpNewHTTPRequest(ctx, endpoint, payload, sessionID)
	if err != nil {
		return fmt.Errorf("failed to initialize HTTP request to '%s': %w", endpoint, err)
	}
	resp, err := mcpClientHTTPClient.Do(req)
	if err != nil {
		return mcpTransportError(endpoint, err)
	}
	defer resp.Body.Close()
	body, _ := mcpReadBody(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream MCP server '%s' rejected the initialized notification with HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// resolveMCPEndpointURL resolves the legacy SSE `endpoint` event data, which may
// be relative to the SSE stream URL.
func resolveMCPEndpointURL(baseURL, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty endpoint event data")
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}
