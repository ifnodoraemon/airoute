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
	reqSnap, _ := c.Get(ContextKeyRequestSnapshot)
	reqStr, _ := reqSnap.(string)

	tc.Record(&storage.RequestTraceRecord{
		TraceID:         middleware.GetTraceID(c),
		ChatID:          c.GetString("chat_id"),
		SessionID:       c.GetString(router.ContextKeySessionID),
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

// RecordTraceFromError is a convenience wrapper for dispatch failures.
func RecordTraceFromError(c *gin.Context, modelName, sessionID string, statusCode int, err error, duration time.Duration) {
	if c == nil {
		return
	}
	if sessionID != "" {
		c.Set(router.ContextKeySessionID, sessionID)
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	RecordTrace(c, modelName, "", statusCode, msg, "", duration)
}

// respondWithTraceError writes an OpenAI-shaped error and archives the snapshot.
func respondWithTraceError(c *gin.Context, modelName string, statusCode int, message, errType, code string, duration time.Duration) {
	RecordTrace(c, modelName, "", statusCode, message, "", duration)
	RespondOpenAIError(c, statusCode, message, errType, code)
}
