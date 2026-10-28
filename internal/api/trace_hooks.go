package api

import (
	"bytes"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ContextKeyRequestSnapshot carries the raw (size-limited) request body so the
// trace archiver can persist it on failure without re-reading the stream.
const ContextKeyRequestSnapshot = "request_snapshot"

// ContextKeyChatID carries the upstream response id of the served request so the
// trace archiver can persist it.
const ContextKeyChatID = "chat_id"

// ContextKeyLLMSessionID carries the client-visible LLM affinity session id.
//
// The routing context key (router.ContextKeySessionID) now holds a value that is
// namespaced by the authenticated caller (A18); traces must show the session the
// client actually sent, so handlers publish that raw value here.
const ContextKeyLLMSessionID = "llm_session_id"

// ContextKeyTraceRecorded marks that a trace snapshot for this request was
// already archived. One request has exactly one trace row: dispatch failures
// record their trace before the usage recorder runs, and the 499 audit path can
// be reached more than once (forced flip plus a repeated recordUsage). Without
// the flag the console showed duplicate rows for one trace_id.
const ContextKeyTraceRecorded = "trace_recorded"

// traceSessionID resolves the session id to persist in a trace record.
func traceSessionID(c *gin.Context) string {
	if s := c.GetString(ContextKeyLLMSessionID); s != "" {
		return s
	}
	return c.GetString(router.ContextKeySessionID)
}

// snapshotMaxBytes caps how much of the inbound body is retained per request.
const snapshotMaxBytes = 32 * 1024

// replayBody presents "bytes already consumed for tracing" followed by the
// still-unread remainder of the original body, and keeps the original Close.
type replayBody struct {
	io.Reader
	closer io.Closer
}

func (b *replayBody) Close() error {
	if b.closer == nil {
		return nil
	}
	return b.closer.Close()
}

// CaptureRequestSnapshot stores a truncated copy of the request body in the gin
// context. It is a no-op when trace capture is disabled.
//
// Only the first snapshotMaxBytes are read and buffered; the rest stays in the
// network stream and is presented to the handler through a chained reader. The
// previous implementation read the whole body (600-750 KB on a typical chat
// request) to keep 32 KB of it, and the full byte slice stayed alive until the
// response stream finished — hundreds of megabytes under concurrency.
func CaptureRequestSnapshot(c *gin.Context) {
	if storage.GlobalTraceCapture == nil || c == nil || c.Request == nil || c.Request.Body == nil {
		return
	}
	original := c.Request.Body
	head, err := io.ReadAll(io.LimitReader(original, snapshotMaxBytes))
	if len(head) > 0 {
		c.Set(ContextKeyRequestSnapshot, string(head))
	}
	// Put the consumed bytes back in front of the unread remainder so the
	// handler still sees the complete request body.
	c.Request.Body = &replayBody{Reader: io.MultiReader(bytes.NewReader(head), original), closer: original}
	if err != nil && len(head) == 0 {
		return
	}
}

// RecordTrace archives a redacted snapshot for the given response status. The
// request snippet comes from CaptureRequestSnapshot; the response snippet is
// supplied by the caller when it is cheap to do so.
func RecordTrace(c *gin.Context, modelName, channel string, statusCode int, errMsg, responseSnippet string, duration time.Duration) {
	tc := storage.GlobalTraceCapture
	if tc == nil || c == nil || !tc.Enabled(statusCode) {
		return
	}
	// Dedupe per request. The flag is set only once a snapshot is actually
	// eligible, so a caller that arrives before capture is enabled does not burn
	// the slot. Callers keep their richer message: a dispatch error recorded
	// first wins over the synthesized 499 explanation from the usage recorder.
	if _, recorded := c.Get(ContextKeyTraceRecorded); recorded {
		return
	}
	c.Set(ContextKeyTraceRecorded, true)

	reqSnap, _ := c.Get(ContextKeyRequestSnapshot)
	reqStr, _ := reqSnap.(string)

	tc.Record(&storage.RequestTraceRecord{
		TraceID:         middleware.GetTraceID(c),
		ChatID:          c.GetString(ContextKeyChatID),
		SessionID:       traceSessionID(c),
		APIKey:          getRequestAPIKey(c),
		TenantID:        c.GetString(middleware.ContextKeyTenant),
		Model:           modelName,
		Channel:         channel,
		StatusCode:      statusCode,
		ErrorMessage:    strings.TrimSpace(errMsg),
		RequestSnippet:  reqStr,
		ResponseSnippet: responseSnippet,
		DurationMs:      duration.Milliseconds(),
	})
}
