package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// useNativeStream inspects the first committed event before emitting any
// gateway-generated frames. Translated streams keep their existing renderer.
func (h *Handler) useNativeStream(c *gin.Context, ctx context.Context, req *model.ChatCompletionRequest, in <-chan *model.StreamEvent, session string, start time.Time) (<-chan *model.StreamEvent, bool) {
	if req.Native == nil {
		return in, false
	}
	var first *model.StreamEvent
	select {
	case first = <-in:
	case <-ctx.Done():
		return in, false
	}
	if first == nil {
		return in, false
	}
	if first.NativeProtocol != "" {
		h.forwardNativeStream(c, req, in, first, session, start)
		return nil, true
	}
	return prependStream(ctx, first, in), false
}

func prependStream(ctx context.Context, first *model.StreamEvent, in <-chan *model.StreamEvent) <-chan *model.StreamEvent {
	out := make(chan *model.StreamEvent, 1)
	out <- first
	go func() {
		defer close(out)
		for event := range in {
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

type nativeStreamStats struct {
	id, identity, channel                string
	input, output, cached, writes, chars int
	ttft                                 time.Duration
	done                                 bool
	err                                  error
	recorded                             bool
	settle                               func()
}

func (s *nativeStreamStats) observe(event *model.StreamEvent, start time.Time) {
	if event.Channel != "" {
		s.channel = event.Channel
	}
	if event.NativeIdentity != "" {
		s.identity = event.NativeIdentity
	}
	s.done = s.done || event.IsDone
	if event.Err != nil {
		s.err = event.Err
	}
	if s.ttft == 0 && event.NativeContent {
		s.ttft = time.Since(start)
		telemetry.GlobalMetrics.RecordTTFT(s.ttft)
	}
	s.chars += event.NativeOutputChars
	telemetry.GlobalLiveMetrics.AddChars(event.NativeOutputChars)
	if chunk := event.Chunk; chunk != nil {
		if chunk.ID != "" {
			s.id = chunk.ID
		}
		if usage := chunk.Usage; usage != nil {
			s.input, s.output = max(s.input, usage.PromptTokens), max(s.output, usage.CompletionTokens)
			s.cached, s.writes = max(s.cached, usage.GetCachedTokens()), max(s.writes, usage.GetCacheCreationTokens())
		}
	}
}

func (h *Handler) forwardNativeStream(c *gin.Context, req *model.ChatCompletionRequest, in <-chan *model.StreamEvent, first *model.StreamEvent, session string, start time.Time) {
	flusher, ok := InitSSEStream(c)
	if !ok {
		RespondOpenAIError(c, http.StatusInternalServerError, "Streaming unsupported", "server_error", "streaming_unsupported")
		return
	}
	stats := &nativeStreamStats{}
	defer stats.finish(c, req, session, start)
	clientGone := false
	doneCh := c.Request.Context().Done()
	write := func(event *model.StreamEvent) bool {
		h.prepareNativeEvent(c, req, event)
		stats.observe(event, start)
		if req.Native.Protocol == model.NativeResponses && event.IsDone && event.Err == nil {
			persistNativeResponseContext(c, stats.id, stats.identity, stats.input+stats.output)
		}
		if !clientGone {
			if len(event.NativeEvent) > 0 {
				if !stats.recorded && req.Native.Protocol == model.NativeChat && bytes.Equal(bytes.TrimSpace(event.NativeEvent), []byte("data: [DONE]")) {
					cost, saved, cached := stats.prepare(c, req, session, start)
					writeStreamUsageTrailer(c.Writer, flusher, cost, saved, cached)
				}
				_, _ = c.Writer.Write(event.NativeEvent)
			} else if event.Err != nil {
				writeNativeStreamError(c.Writer, req.Native.Protocol, event.Err)
			}
			flusher.Flush()
		}
		return event.Err == nil
	}
	if !write(first) {
		return
	}
	for {
		select {
		case <-doneCh:
			if !upstreamDrainingEnabled() {
				return
			}
			clientGone, doneCh = true, nil
			markAbortedDrain(0)
		case event, open := <-in:
			if !open {
				return
			}
			if event != nil && !write(event) {
				return
			}
		}
	}
}

func (s *nativeStreamStats) finish(c *gin.Context, req *model.ChatCompletionRequest, session string, start time.Time) {
	if !s.recorded {
		s.prepare(c, req, session, start)
	}
	if s.settle != nil {
		s.settle()
	}
	if req.Native.AfterRecord != nil {
		req.Native.AfterRecord()
	}
}

func (s *nativeStreamStats) prepare(c *gin.Context, req *model.ChatCompletionRequest, session string, start time.Time) (float64, float64, int) {
	s.recorded = true
	if req.Native.ShouldRecord != nil && !req.Native.ShouldRecord() {
		return 0, 0, 0
	}
	status := http.StatusOK
	if c.Request.Context().Err() != nil {
		status = statusClientClosed
		markAbortedDrain(s.output)
	} else if s.err != nil || !s.done {
		status = http.StatusBadGateway
		if s.err == nil {
			s.err = io.ErrUnexpectedEOF
		}
	}
	source := "upstream"
	if status == http.StatusOK || status == statusClientClosed {
		if s.input == 0 && estimateCanonicalPromptTokens(req) > 0 {
			s.input, source = streamFallbackPromptTokens(req), usageSourceEstimated
		}
		if s.output == 0 && s.chars > 0 {
			s.output, source = estimatedStreamCompletionTokens(s.chars), usageSourceEstimated
		}
	}
	cost, saved, cached, settle := RecordUsageDeferred(c, AuditRecordParams{
		BillingID: req.Native.BillingID,
		ChatID:    s.id, Channel: s.channel, SessionID: session, Model: req.Model,
		PromptTokens: s.input, CompletionTokens: s.output, CachedTokens: s.cached, CacheCreationTokens: s.writes,
		Duration: time.Since(start), TTFT: s.ttft, StatusCode: status, UsageSource: source, LiveTracked: true,
		ErrorMessage: sanitizeErrorMessage(s.err),
	})
	s.settle = settle
	if status == http.StatusOK {
		RecordSuccessTrace(c, req.Model, s.channel, time.Since(start), nil)
	}
	return cost, saved, cached
}

func writeNativeStreamError(w io.Writer, protocol string, err error) {
	message := sanitizeErrorMessage(err)
	var body []byte
	switch protocol {
	case model.NativeGeminiInteractions:
		body, _ = json.Marshal(gin.H{"event_type": "error", "error": gin.H{"message": message, "code": "INTERNAL"}})
		_, _ = fmt.Fprint(w, "event: error\n")
	case model.NativeGemini:
		body, _ = json.Marshal(gin.H{"error": gin.H{"message": message, "status": "UNAVAILABLE", "code": 502}})
	case model.NativeResponses:
		body, _ = json.Marshal(gin.H{"type": "response.failed", "response": gin.H{"status": "failed", "error": gin.H{"message": message, "code": "stream_error"}}})
		_, _ = fmt.Fprint(w, "event: response.failed\n")
	case model.NativeMessages:
		body, _ = json.Marshal(gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": message}})
		_, _ = fmt.Fprint(w, "event: error\n")
	default:
		body, _ = json.Marshal(gin.H{"error": gin.H{"message": message, "type": "server_error", "code": "stream_error"}})
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
}

func (h *Handler) prepareNativeEvent(c *gin.Context, req *model.ChatCompletionRequest, event *model.StreamEvent) {
	if event.NativeProtocol == model.NativeGemini {
		if err := h.retainGeneratedFiles(c, req.Model, event.NativeIdentity, event.NativeEvent); err != nil {
			event.Err = err
			event.NativeEvent = nil
		}
	}
	if event.BeforeForward != nil {
		if err := event.BeforeForward(); err != nil {
			event.Err = err
			event.NativeEvent = nil
		}
	}
}
