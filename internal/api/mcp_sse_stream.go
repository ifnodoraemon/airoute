package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

// MCP client SSE stream reader and transport selection, extracted from
// mcp_client.go to keep the RPC helpers file under the size red line.

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