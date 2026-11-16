package provider

import (
	"bufio"
	"fmt"
	"io"
)

// maxChatBodyBytes bounds a single upstream chat response body (unary) or a
// single SSE line (streaming). A malicious or broken relay must not exhaust
// gateway memory with one enormous body/line; the native and modality paths
// already cap their reads, the chat main path used not to.
const maxChatBodyBytes = 16 << 20

// maxChatErrorBodyBytes bounds an upstream error body read for diagnostics.
const maxChatErrorBodyBytes = 1 << 20

// readCappedLine reads one '\n'-terminated line like bufio.Reader.ReadBytes but
// refuses to buffer more than maxChatBodyBytes, returning an error instead of
// growing without bound on a single enormous SSE line.
func readCappedLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxChatBodyBytes {
			return nil, fmt.Errorf("upstream SSE line exceeds 16 MiB")
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, err
	}
}

// readChatBody reads an entire unary chat body up to the memory cap.
func readChatBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxChatBodyBytes))
}