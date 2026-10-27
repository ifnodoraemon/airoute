package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// stdio transport for the outbound MCP client.
//
// A stdio MCP server is a child process speaking newline-delimited JSON-RPC on
// its stdin/stdout. The lifecycle is the same as the HTTP transports: initialize,
// notifications/initialized, tools/call. The previous implementation wrote a bare
// tools/call to stdin and closed it, so a real server never even got past its
// handshake.

// mcpStdioStderrBuffer keeps only a bounded tail of the child's stderr for
// diagnostics; an untrusted process must not be able to exhaust gateway memory.
type mcpStdioStderrBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	limit  int
	truncT bool
}

func newMCPStdioStderrBuffer(limit int) *mcpStdioStderrBuffer {
	return &mcpStdioStderrBuffer{limit: limit}
}

func (b *mcpStdioStderrBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(p)
	if b.buf.Len()+len(p) > b.limit {
		keep := b.limit - b.buf.Len()
		if keep > 0 {
			b.buf.Write(p[:keep])
		}
		b.truncT = true
		return written, nil
	}
	b.buf.Write(p)
	return written, nil
}

func (b *mcpStdioStderrBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncT {
		return strings.TrimSpace(b.buf.String()) + " (stderr truncated)"
	}
	return strings.TrimSpace(b.buf.String())
}

// executeStdioMCPTool runs the full stdio lifecycle against cmdParts and returns
// the tool result text and isError flag.
func executeStdioMCPTool(ctx context.Context, cmdParts []string, toolName string, args map[string]interface{}) (string, bool) {
	if len(cmdParts) == 0 || strings.TrimSpace(cmdParts[0]) == "" {
		return "MCP stdio server command is empty", true
	}

	callCtx, cancel := mcpCallContext(ctx)
	defer cancel()

	cmd := exec.CommandContext(callCtx, cmdParts[0], cmdParts[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Sprintf("Stdio process setup error: %v", err), true
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Sprintf("Stdio process setup error: %v", err), true
	}
	stderr := newMCPStdioStderrBuffer(64 << 10)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("Stdio process execution error: %v", err), true
	}

	waited := false
	defer func() {
		_ = stdin.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	reader := bufio.NewReaderSize(stdout, 64<<10)

	// 1. initialize
	initID := mcpNextClientID()
	initPayload, err := mcpBuildRequest(initID, "initialize", mcpInitializePayload())
	if err != nil {
		return fmt.Sprintf("Failed to serialize MCP initialize request: %v", err), true
	}
	if err := mcpWriteStdioLine(stdin, initPayload); err != nil {
		return fmt.Sprintf("Stdio process is not writable: %v, stderr: %s", err, stderr.String()), true
	}
	initResp, err := mcpReadStdioResponse(reader, initID)
	if err != nil {
		return fmt.Sprintf("Stdio MCP server did not complete initialize: %v, stderr: %s", err, stderr.String()), true
	}
	if initResp.Error != nil {
		return mcpRPCErrorText(initResp.Error), true
	}

	// 2. notifications/initialized
	notifyPayload, err := mcpBuildRequest(0, "notifications/initialized", nil)
	if err != nil {
		return fmt.Sprintf("Failed to serialize MCP initialized notification: %v", err), true
	}
	if err := mcpWriteStdioLine(stdin, notifyPayload); err != nil {
		return fmt.Sprintf("Stdio process is not writable: %v, stderr: %s", err, stderr.String()), true
	}

	// 3. tools/call
	callID := mcpNextClientID()
	callPayload, err := mcpBuildRequest(callID, "tools/call", mcpToolsCallPayload(toolName, args))
	if err != nil {
		return fmt.Sprintf("Failed to serialize MCP tools/call request: %v", err), true
	}
	if err := mcpWriteStdioLine(stdin, callPayload); err != nil {
		return fmt.Sprintf("Stdio process is not writable: %v, stderr: %s", err, stderr.String()), true
	}
	callResp, err := mcpReadStdioResponse(reader, callID)
	if err != nil {
		return fmt.Sprintf("Stdio MCP server did not answer tools/call: %v, stderr: %s", err, stderr.String()), true
	}

	// Close stdin and reap the child so no zombie or held pipe is left behind.
	_ = stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		waited = true
		// A server may exit non-zero after answering; the reply we already have
		// is still valid, so the exit status is not itself a tool failure.
		_ = waitErr
	case <-callCtx.Done():
		_ = cmd.Process.Kill()
		<-done
		waited = true
		return fmt.Sprintf("Stdio MCP server timed out: %v", callCtx.Err()), true
	}

	if callResp.Error != nil {
		return mcpRPCErrorText(callResp.Error), true
	}
	text, isErr := mcpParseToolResult(callResp.Result)
	return text, isErr
}

// mcpWriteStdioLine writes one newline-delimited JSON-RPC message.
func mcpWriteStdioLine(w io.Writer, payload []byte) error {
	buf := make([]byte, 0, len(payload)+1)
	buf = append(buf, payload...)
	buf = append(buf, '\n')
	_, err := w.Write(buf)
	return err
}

// mcpReadStdioResponse reads newline-delimited messages until the one matching id
// arrives, skipping server log lines and notifications.
func mcpReadStdioResponse(reader *bufio.Reader, id int64) (*mcpClientResponse, error) {
	var total int64
	for {
		line, err := reader.ReadString('\n')
		total += int64(len(line))
		if total > mcpClientMaxResponseBytes {
			return nil, fmt.Errorf("stdio MCP server exceeded the %d byte response limit", mcpClientMaxResponseBytes)
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			if responses, parseErr := mcpDecodeRPCResponses([]byte(trimmed)); parseErr == nil {
				if reply := mcpPickResponse(responses, id); reply != nil {
					return reply, nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("stdio MCP server closed stdout without a response for request id %d", id)
			}
			return nil, err
		}
	}
}
