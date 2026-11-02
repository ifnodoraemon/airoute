package provider

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func nativeStreaming(ctx context.Context, client *http.Client, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, bool, error) {
	r, err := nativeHTTPRequest(ctx, req, channel)
	if err != nil {
		return nil, true, err
	}
	resp, err := client.Do(r)
	if err != nil {
		return nil, true, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		done, err := readNativeError(resp, req, channel)
		return nil, done, err
	}
	if req.Native.Protocol == model.NativeResponses || req.Native.Protocol == model.NativeMessages {
		peek, compatible, err := peekNativeStream(resp.Body, req.Native.Protocol)
		if err != nil || !compatible {
			resp.Body.Close()
			return nil, err != nil, err
		}
		resp.Body = peek
	}
	out := make(chan *model.StreamEvent, 64)
	go readNativeStream(ctx, out, resp.Body, req, channel)
	return out, true, nil
}

type replayNativeBody struct {
	io.Reader
	io.Closer
}

func peekNativeStream(body io.ReadCloser, protocol string) (io.ReadCloser, bool, error) {
	reader := bufio.NewReader(body)
	var seen bytes.Buffer
	for seen.Len() < 1<<20 {
		line, err := reader.ReadBytes('\n')
		seen.Write(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			return &replayNativeBody{Reader: io.MultiReader(bytes.NewReader(seen.Bytes()), reader), Closer: body}, !nativeReplyWrongProtocol(protocol, data), nil
		}
		if err != nil {
			if err == io.EOF {
				return &replayNativeBody{Reader: bytes.NewReader(seen.Bytes()), Closer: body}, true, nil
			}
			return nil, false, err
		}
	}
	return nil, false, fmt.Errorf("native SSE preamble exceeds 1 MiB")
}

func readNativeStream(ctx context.Context, out chan<- *model.StreamEvent, body io.ReadCloser, req *model.ChatCompletionRequest, channel *model.ChannelConfig) {
	defer close(out)
	defer body.Close()
	// Closing the body unblocks ReadString even for custom transports that don't
	// tie their reader to the HTTP request's cancellation.
	stopClose := context.AfterFunc(ctx, func() { _ = body.Close() })
	defer stopClose()
	send := func(event *model.StreamEvent) bool {
		select {
		case out <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}
	reader := bufio.NewReader(body)
	var frame []byte
	terminal := false
	for {
		line, err := reader.ReadBytes('\n')
		if len(frame)+len(line) > 16<<20 {
			send(&model.StreamEvent{Err: fmt.Errorf("upstream SSE event exceeds 16 MiB")})
			return
		}
		frame = append(frame, line...)
		if len(bytes.TrimSpace(line)) == 0 || err != nil {
			if len(frame) > 0 {
				event := nativeStreamEvent(req.Native.Protocol, frame, req.Model, channel.Name)
				event.NativeIdentity = model.UpstreamIdentity(channel)
				terminal = terminal || event.IsDone
				if !send(event) || event.Err != nil {
					return
				}
				frame = nil
			}
		}
		if err != nil {
			if ctx.Err() == nil && (err != io.EOF || !terminal) {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				send(&model.StreamEvent{Err: fmt.Errorf("read native stream: %w", err)})
			}
			return
		}
	}
}
