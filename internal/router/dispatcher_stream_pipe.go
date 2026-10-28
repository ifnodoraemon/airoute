package router

import (
	"context"
	"errors"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// errStreamTruncated marks a stream whose upstream channel closed without a
// terminal marker ([DONE] / a finish_reason). It is a channel-health signal, not
// something the client is told about directly: the stream simply ends, as before.
var errStreamTruncated = errors.New("upstream closed the stream before it completed")

// recordStreamFailure accounts for an upstream failure that surfaced AFTER the
// stream was committed to the client.
//
// The commit-time RecordSuccess is only valid as far as "this channel produced a
// first token". A channel that then dies mid-stream ("connects, emits one token,
// always truncates long output") is exactly the degradation the breaker exists
// for, yet the old pipe only forwarded event.Err to the audit layer and never
// wrote it back to the breaker, so such a channel stayed CLOSED forever while the
// same behaviour tripped it in three unary attempts (B-02).
func (d *Dispatcher) recordStreamFailure(ctx context.Context, chName string, err error, attempt int) {
	if err == nil {
		return
	}
	// A client that hung up is not an upstream problem: the request context being
	// done explains both the cancellation and the truncated stream.
	if ctx.Err() != nil {
		return
	}
	// Cancellation inside the provider (e.g. an aborted attempt) is ambiguous at
	// best; never charge the channel for it.
	if errors.Is(err, context.Canceled) {
		return
	}
	// A payload error the caller caused is not a channel health signal even when
	// it arrives mid-stream.
	if provider.IsFatalPayloadError(err) {
		return
	}
	// A status-carrying error keeps the shared classifier's meaning: 429 is
	// backpressure from a healthy upstream and a node-specific 404 only means
	// another node may serve the model, so neither belongs in the breaker.
	if _, typed := provider.AsUpstreamError(err); typed && !provider.IsChannelFailure(err) {
		return
	}
	// Everything else (transport read aborts, upstream error frames, a stream
	// closed without a terminal marker) is an upstream side abort after the first
	// token reached the client: exactly the failure mode the breaker exists for.
	d.recordChannelFailure(ctx, chName, err, attempt)
}

// pipeStreamEvents pipes events from upstream stream channel to client channel with cancellation guard.
//
// onFailure, when non-nil, is called at most once per stream for evidence of an
// upstream-side failure after the commit: an event carrying Err, or a channel
// that closes without a terminal marker. Client cancellation (ctx.Done) never
// calls it.
func pipeStreamEvents(ctx context.Context, outChan chan<- *model.StreamEvent, preamble []*model.StreamEvent, in <-chan *model.StreamEvent, trail string, cancel context.CancelFunc, onFailure func(error)) {
	defer cancel()
	defer close(outChan)

	// One stream is one health sample: an error frame followed by the channel
	// closing must not be counted twice.
	reported := false
	report := func(err error) {
		if reported || onFailure == nil {
			return
		}
		reported = true
		onFailure(err)
	}

	done := false
	for _, event := range preamble {
		if streamEventIsTerminal(event) {
			done = true
		}
		event.Channel = trail
		select {
		case outChan <- event:
		case <-ctx.Done():
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-in:
			if !ok {
				// Closing the input without a terminal marker means the upstream
				// cut the stream off (or its read failed silently); that is a
				// channel failure, unlike a client that went away (ctx.Done above).
				if !done {
					report(errStreamTruncated)
				}
				return
			}
			if event == nil {
				continue
			}
			if event.Err != nil {
				report(event.Err)
			}
			if streamEventIsTerminal(event) {
				done = true
			}
			event.Channel = trail
			select {
			case outChan <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

func streamEventHasContent(event *model.StreamEvent) bool {
	if event == nil || event.Chunk == nil {
		return event != nil && (event.NativeAnthropicType == "content_block_delta" || event.NativeAnthropicType == "content_block_start")
	}
	for _, choice := range event.Chunk.Choices {
		if choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" || len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

// streamEventIsTerminal reports whether an event proves the upstream reached the
// end of its stream. Every in-tree provider sends an explicit IsDone frame
// ([DONE] / message_stop); a finish_reason is accepted as the fallback marker for
// OpenAI-compatible upstreams that close right after the final chunk.
func streamEventIsTerminal(event *model.StreamEvent) bool {
	if event == nil {
		return false
	}
	if event.IsDone {
		return true
	}
	if event.Chunk == nil {
		return false
	}
	for _, choice := range event.Chunk.Choices {
		if choice.FinishReason != nil {
			return true
		}
	}
	return false
}
