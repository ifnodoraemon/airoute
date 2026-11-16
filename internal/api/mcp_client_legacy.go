package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Legacy HTTP/SSE MCP transport (MCP 2024-11-05), extracted from
// mcp_client_http.go to keep the streamable-HTTP file under the size red line.

func callLegacySSEMCPTool(ctx context.Context, endpoint, toolName string, args map[string]interface{}) (string, bool, error) {
	callCtx, cancel := mcpCallContext(ctx)
	defer cancel()

	getReq, err := http.NewRequestWithContext(callCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", true, fmt.Errorf("failed to initialize HTTP request to '%s': %w", endpoint, err)
	}
	getReq.Header.Set("Accept", "text/event-stream")
	getReq.Header.Set("User-Agent", "Airoute-MCP-Client/"+mcpClientVersion)

	resp, err := mcpClientHTTPClient.Do(getReq)
	if err != nil {
		return "", true, mcpFallbackError("%v", mcpTransportError(endpoint, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		if mcpFallbackStatus(resp.StatusCode) {
			return "", true, mcpFallbackError("upstream MCP server '%s' is not an SSE endpoint (HTTP %d)", endpoint, resp.StatusCode)
		}
		return "", true, fmt.Errorf("upstream MCP server '%s' returned HTTP %d", endpoint, resp.StatusCode)
	}

	stream := newMCPSSEStream(resp.Body)
	defer stream.close()

	// The first event must advertise the message endpoint. Servers that answer
	// JSON here are streamable-HTTP servers, so falling back is safe.
	postURL := ""
	for postURL == "" {
		event, err := stream.next(callCtx)
		if err != nil {
			return "", true, mcpFallbackError("MCP server '%s' did not advertise a legacy SSE endpoint: %v", endpoint, err)
		}
		if event.Event != "endpoint" {
			continue
		}
		postURL, err = resolveMCPEndpointURL(endpoint, event.Data)
		if err != nil {
			return "", true, fmt.Errorf("MCP server '%s' sent an invalid endpoint event: %w", endpoint, err)
		}
	}

	// 1. initialize
	initID := mcpNextClientID()
	payload, err := mcpBuildRequest(initID, "initialize", mcpInitializePayload())
	if err != nil {
		return "", true, fmt.Errorf("failed to serialize MCP initialize request: %w", err)
	}
	initResp, err := legacyPostExchange(callCtx, postURL, payload, initID, stream)
	if err != nil {
		return "", true, err
	}
	if initResp.Error != nil {
		return mcpRPCErrorText(initResp.Error), true, nil
	}
	if len(initResp.Result) == 0 {
		return "", true, fmt.Errorf("MCP server '%s' returned an empty initialize result", endpoint)
	}

	// 2. notifications/initialized
	notifyPayload, err := mcpBuildRequest(0, "notifications/initialized", nil)
	if err != nil {
		return "", true, fmt.Errorf("failed to serialize MCP initialized notification: %w", err)
	}
	if err := legacyPostNotify(callCtx, postURL, notifyPayload); err != nil {
		return "", true, err
	}

	// 3. tools/call
	callID := mcpNextClientID()
	payload, err = mcpBuildRequest(callID, "tools/call", mcpToolsCallPayload(toolName, args))
	if err != nil {
		return "", true, fmt.Errorf("failed to serialize MCP tools/call request: %w", err)
	}
	callResp, err := legacyPostExchange(callCtx, postURL, payload, callID, stream)
	if err != nil {
		return "", true, err
	}
	if callResp.Error != nil {
		return mcpRPCErrorText(callResp.Error), true, nil
	}
	text, isErr := mcpParseToolResult(callResp.Result)
	return text, isErr, nil
}

// legacyPostExchange POSTs a request and waits for the matching reply, either in
// the POST response body or as a `message` event on the SSE stream.
func legacyPostExchange(ctx context.Context, postURL string, payload []byte, id int64, stream *mcpSSEStream) (*mcpClientResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize HTTP request to '%s': %w", postURL, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := mcpClientHTTPClient.Do(req)
	if err != nil {
		return nil, mcpTransportError(postURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("legacy MCP server rejected the message with HTTP %d", resp.StatusCode)
	}

	body, err := mcpReadBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading legacy MCP POST response: %w", err)
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if responses, parseErr := mcpDecodeRPCResponses(body); parseErr == nil {
			if reply := mcpPickResponse(responses, id); reply != nil {
				return reply, nil
			}
		}
	}

	// The reply arrives on the SSE stream, potentially interleaved with
	// notifications and keep-alives.
	for {
		event, err := stream.next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("legacy MCP server closed the event stream without a response for request id %d", id)
			}
			return nil, mcpTransportError(postURL, err)
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

// legacyPostNotify POSTs a JSON-RPC notification (no reply expected).
func legacyPostNotify(ctx context.Context, postURL string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to initialize HTTP request to '%s': %w", postURL, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := mcpClientHTTPClient.Do(req)
	if err != nil {
		return mcpTransportError(postURL, err)
	}
	defer resp.Body.Close()
	_, _ = mcpReadBody(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("legacy MCP server rejected the initialized notification with HTTP %d", resp.StatusCode)
	}
	return nil
}