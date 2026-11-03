package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// This file implements the client half of MCP: the gateway acting as an MCP
// client towards the external servers an operator registered in the MCP plaza.
// The previous implementation sent a bare tools/call POST (or wrote it to a
// stdio process and closed stdin), skipping the MCP lifecycle — initialize,
// protocol-version negotiation and the initialized notification — and could not
// read SSE replies. Real MCP servers reject that. The three supported transports
// live in mcp_client_http.go (streamable HTTP and legacy HTTP/SSE) and
// mcp_client_stdio.go.

const (
	// mcpClientProtocolVersion is the revision this client implements. The
	// task's contract is explicit about 2024-11-05; servers negotiate down in
	// their initialize reply.
	mcpClientProtocolVersion = "2024-11-05"
	// mcpClientName / mcpClientVersion identify the gateway in initialize.
	mcpClientName    = "airoute-gateway"
	mcpClientVersion = "1.0.0"
	// mcpClientMaxResponseBytes caps everything read back from an untrusted MCP
	// server (HTTP body, SSE stream or stdio line stream).
	mcpClientMaxResponseBytes int64 = 8 << 20
	// mcpClientTimeout bounds one complete tool call across all transports.
	mcpClientTimeout = 30 * time.Second
)

// mcpClientID is a monotonically increasing JSON-RPC request id. Numeric ids
// match how most MCP servers echo ids back, and mcpIDMatches accepts both the
// numeric and string form.
var mcpClientID atomic.Int64

func mcpNextClientID() int64 { return mcpClientID.Add(1) }

// mcpClientRPCError is a JSON-RPC error object returned by a server.
type mcpClientRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// mcpClientResponse is a JSON-RPC 2.0 response (or notification) from a server.
type mcpClientResponse struct {
	JSONRPC string             `json:"jsonrpc"`
	ID      json.RawMessage    `json:"id,omitempty"`
	Result  json.RawMessage    `json:"result,omitempty"`
	Error   *mcpClientRPCError `json:"error,omitempty"`
	Method  string             `json:"method,omitempty"`
	Params  json.RawMessage    `json:"params,omitempty"`
}

// mcpClientToolResult is the result payload of tools/call.
type mcpClientToolResult struct {
	Content           []json.RawMessage `json:"content"`
	IsError           bool              `json:"isError"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
}

// errMCPFallback marks a transport/setup failure that justifies trying the other
// HTTP transport. Failures after a server accepted a message (protocol errors,
// tool errors) are never fallback-safe: retrying them would replay side effects.
type errMCPFallback struct{ err error }

func (e *errMCPFallback) Error() string { return e.err.Error() }
func (e *errMCPFallback) Unwrap() error { return e.err }

func mcpFallbackError(format string, args ...interface{}) error {
	return &errMCPFallback{err: fmt.Errorf(format, args...)}
}

func isMCPFallbackError(err error) bool {
	var target *errMCPFallback
	return errors.As(err, &target)
}

// mcpCallContext bounds a client call to 30s unless the caller already set a
// (possibly shorter) deadline. Either way the returned cancel must be called.
func mcpCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if _, ok := parent.Deadline(); ok {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, mcpClientTimeout)
}

// mcpClientHTTPClient is shared between transports so connections to the same
// server are reused. Deadlines come from the request context.
var mcpClientHTTPClient = &http.Client{}

// mcpBuildRequest renders a JSON-RPC request. A zero id makes it a notification.
func mcpBuildRequest(id int64, method string, params interface{}) ([]byte, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if id != 0 {
		payload["id"] = id
	}
	if params != nil {
		payload["params"] = params
	}
	return json.Marshal(payload)
}

// mcpInitializePayload builds the initialize params the MCP lifecycle requires.
func mcpInitializePayload() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": mcpClientProtocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    mcpClientName,
			"version": mcpClientVersion,
		},
	}
}

// mcpToolsCallPayload builds tools/call params.
func mcpToolsCallPayload(toolName string, args map[string]interface{}) map[string]interface{} {
	if args == nil {
		args = map[string]interface{}{}
	}
	return map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	}
}

// mcpIDMatches reports whether a response id equals the numeric request id,
// accepting both the numeric and quoted-string forms servers use.
func mcpIDMatches(raw json.RawMessage, id int64) bool {
	if len(raw) == 0 {
		return false
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch typed := v.(type) {
	case float64:
		return int64(typed) == id
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return err == nil && parsed == id
	default:
		return false
	}
}

// mcpDecodeRPCResponses decodes a single JSON-RPC response or a JSON-RPC batch.
func mcpDecodeRPCResponses(body []byte) ([]*mcpClientResponse, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if trimmed[0] == '[' {
		var batch []*mcpClientResponse
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			return nil, err
		}
		return batch, nil
	}
	var single mcpClientResponse
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, err
	}
	return []*mcpClientResponse{&single}, nil
}

// mcpPickResponse returns the response matching id, skipping notifications.
func mcpPickResponse(responses []*mcpClientResponse, id int64) *mcpClientResponse {
	for _, resp := range responses {
		if resp == nil {
			continue
		}
		if mcpIDMatches(resp.ID, id) {
			return resp
		}
	}
	return nil
}

// mcpRPCErrorText renders a server-side JSON-RPC error.
func mcpRPCErrorText(rpcErr *mcpClientRPCError) string {
	if rpcErr == nil {
		return "Unknown MCP error"
	}
	if len(rpcErr.Data) > 0 {
		return fmt.Sprintf("MCP execution error (code %d): %s: %s", rpcErr.Code, rpcErr.Message, string(rpcErr.Data))
	}
	return fmt.Sprintf("MCP execution error (code %d): %s", rpcErr.Code, rpcErr.Message)
}

// mcpParseToolResult turns a tools/call result into text. Text content is
// returned as-is (joined for multiple items); non-text content, structured
// content and unknown result shapes are returned as JSON so nothing is lost.
func mcpParseToolResult(result json.RawMessage) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	var parsed mcpClientToolResult
	if err := json.Unmarshal(result, &parsed); err == nil && (len(parsed.Content) > 0 || parsed.IsError) {
		parts := make([]string, 0, len(parsed.Content))
		for _, raw := range parsed.Content {
			var item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &item); err == nil && (item.Type == "text" || item.Type == "") {
				parts = append(parts, item.Text)
				continue
			}
			// Image/audio/resource content: keep the JSON rather than dropping it.
			if compact, err := json.Marshal(json.RawMessage(raw)); err == nil {
				parts = append(parts, string(compact))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), parsed.IsError
		}
		if len(parsed.StructuredContent) > 0 {
			return string(parsed.StructuredContent), parsed.IsError
		}
		return string(result), parsed.IsError
	}
	return string(result), false
}

// mcpReadBody reads a response body with the 8 MiB cap applied.
func mcpReadBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, mcpClientMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > mcpClientMaxResponseBytes {
		return nil, fmt.Errorf("MCP response exceeds the %d byte limit", mcpClientMaxResponseBytes)
	}
	return data, nil
}

// mcpIsHTTPEndpoint reports whether an endpoint is dialled over HTTP(S).
func mcpIsHTTPEndpoint(endpoint string) bool {
	lower := strings.ToLower(strings.TrimSpace(endpoint))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// mcpSSEEvent is one dispatched Server-Sent Event.
type mcpSSEEvent struct {
	Event string
	Data  string
	ID    string
}

// mcpSSEStream parses an SSE byte stream into events in the background so the
// caller can select on its context while a legacy server keeps the stream open.
type mcpSSEStream struct {
	body   io.ReadCloser
	events chan mcpSSEEvent
	errs   chan error
	done   chan struct{}
	once   sync.Once
}

func newMCPSSEStream(body io.ReadCloser) *mcpSSEStream {
	stream := &mcpSSEStream{
		body:   body,
		events: make(chan mcpSSEEvent),
		errs:   make(chan error, 1),
		done:   make(chan struct{}),
	}
	go stream.run()
	return stream
}

// emit hands an event to the consumer, aborting if the stream was closed.
func (s *mcpSSEStream) emit(event mcpSSEEvent) bool {
	select {
	case s.events <- event:
		return true
	case <-s.done:
		return false
	}
}

func (s *mcpSSEStream) run() {
	defer close(s.events)
	reader := bufio.NewReaderSize(s.body, 64<<10)
	var total int64
	var event, id string
	var data strings.Builder
	hasData := false
	for {
		line, err := reader.ReadString('\n')
		total += int64(len(line))
		if total > mcpClientMaxResponseBytes {
			s.errs <- fmt.Errorf("MCP event stream exceeds the %d byte limit", mcpClientMaxResponseBytes)
			return
		}
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			switch {
			case line == "":
				if hasData && !s.emit(mcpSSEEvent{Event: event, Data: data.String(), ID: id}) {
					return
				}
				event, id, data, hasData = "", "", strings.Builder{}, false
			case strings.HasPrefix(line, ":"):
				// Comment / keep-alive.
			default:
				field, value, found := strings.Cut(line, ":")
				if found {
					value = strings.TrimPrefix(value, " ")
				}
				switch field {
				case "event":
					event = value
				case "data":
					if hasData {
						data.WriteByte('\n')
					}
					data.WriteString(value)
					hasData = true
				case "id":
					id = value
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				if hasData {
					s.emit(mcpSSEEvent{Event: event, Data: data.String(), ID: id})
				}
				s.errs <- io.EOF
				return
			}
			s.errs <- err
			return
		}
	}
}

// next returns the next event, or a terminal error, or ctx.Err() after closing
// the underlying stream.
func (s *mcpSSEStream) next(ctx context.Context) (mcpSSEEvent, error) {
	select {
	case event, ok := <-s.events:
		if ok {
			return event, nil
		}
		select {
		case err := <-s.errs:
			return mcpSSEEvent{}, err
		default:
			return mcpSSEEvent{}, io.EOF
		}
	case <-ctx.Done():
		s.close()
		return mcpSSEEvent{}, ctx.Err()
	}
}

func (s *mcpSSEStream) close() {
	s.once.Do(func() {
		close(s.done)
		_ = s.body.Close()
	})
}

// mcpTransportError normalises connection failures into a stable message while
// preserving the underlying error for logs.
func mcpTransportError(endpoint string, err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("upstream MCP server '%s' timed out: %w", endpoint, err)
	}
	return fmt.Errorf("upstream MCP server '%s' connection failed: %w", endpoint, err)
}

// forwardJSONRPCToolCall calls a tool on an HTTP(S) MCP endpoint, preferring the
// streamable HTTP transport and falling back to the legacy HTTP/SSE transport
// when the server signals it does not speak streamable HTTP. It is exported
// within the package for the historical call site (and tests).
func forwardJSONRPCToolCall(ctx context.Context, endpoint, toolName string, args map[string]interface{}) (string, bool) {
	return forwardHTTPMCPToolCall(ctx, "", endpoint, toolName, args)
}

// forwardHTTPMCPToolCall picks the transport recorded for the server, with a
// safe cross-transport fallback for setup-level (not tool-level) failures.
func forwardHTTPMCPToolCall(ctx context.Context, transport, endpoint, toolName string, args map[string]interface{}) (string, bool) {
	callCtx, cancel := mcpCallContext(ctx)
	defer cancel()

	normalized := strings.ToLower(strings.TrimSpace(transport))
	preferLegacy := normalized == "sse" || normalized == "legacy-sse" || normalized == "http+sse"

	var order []string
	if preferLegacy {
		order = []string{"sse", "streamable"}
	} else {
		order = []string{"streamable", "sse"}
	}

	var fallbackErr error
	for _, candidate := range order {
		var (
			text  string
			isErr bool
			err   error
		)
		if candidate == "sse" {
			text, isErr, err = callLegacySSEMCPTool(callCtx, endpoint, toolName, args)
		} else {
			text, isErr, err = callStreamableHTTPMCPTool(callCtx, endpoint, toolName, args)
		}
		if err == nil {
			return text, isErr
		}
		if !isMCPFallbackError(err) {
			return fmt.Sprintf("%v", err), true
		}
		fallbackErr = err
	}
	return fmt.Sprintf("%v", fallbackErr), true
}
