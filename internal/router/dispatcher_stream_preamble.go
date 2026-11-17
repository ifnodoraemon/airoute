package router

import (
	"context"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// maxPreambleEvents bounds buffering before committing a healthy event stream.
const maxPreambleEvents = 64

// waitStreamPreamble commits on content, a terminal event, or a full preamble.
// Once the buffer is full, continued valid events establish a live stream; all
// buffered frames and the triggering event must reach the downstream intact.
func waitStreamPreamble(ctx, attemptCtx context.Context, stream <-chan *model.StreamEvent, channel string, timeout time.Duration) ([]*model.StreamEvent, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var preamble []*model.StreamEvent
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return nil, fmt.Errorf("channel %s closed stream before a valid token", channel)
			}
			if event == nil {
				continue
			}
			if event.Err != nil {
				return nil, event.Err
			}
			full := len(preamble) >= maxPreambleEvents
			preamble = append(preamble, event)
			if full || streamEventHasContent(event) || streamEventIsTerminal(event) {
				return preamble, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("channel %s timed out waiting for first token (%v)", channel, timeout)
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-attemptCtx.Done():
			if ctx.Err() != nil {
				return nil, context.Cause(ctx)
			}
			return nil, fmt.Errorf("channel %s exceeded its timeout before the first token", channel)
		}
	}
}
