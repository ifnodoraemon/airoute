package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	// ContextKeyTraceID aliases the telemetry constant so whichever package a
	// call site imports, the context key is the same string value.
	ContextKeyTraceID      = telemetry.ContextKeyTraceID
	ContextKeyErrorMessage = "error_message"
	// ContextKeyAuditStatus carries the status persisted to usage_logs when it
	// differs from the wire status. A 499 "client closed request" is synthesized
	// by the gateway: on a streaming response the head was already written as
	// 200, and a unary success is flipped to 499 in the audit record. Without
	// this key the access line claimed a clean 200 for an aborted, billed
	// request, and told the console nothing about the audit status.
	ContextKeyAuditStatus = "audit_status"
	HeaderAirouteTraceID  = "X-Airoute-Trace-ID"
	HeaderRequestID       = "X-Request-ID"
	HeaderTraceID         = "X-Trace-ID"
	HeaderTraceParent     = "traceparent"
)

// GenerateTraceID creates a unique, high-performance distributed trace identifier.
func GenerateTraceID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("tr-%x-%s", time.Now().UnixMicro()%100000000, hex.EncodeToString(b))
}

// TraceMiddleware extracts or generates a distributed TraceID for every incoming request,
// injects it into the context and response headers, and enables end-to-end trace correlation.
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := extractTraceID(c)
		if traceID == "" {
			traceID = GenerateTraceID()
		}

		// Inject into Gin context and standard context.Context
		c.Set(ContextKeyTraceID, traceID)
		c.Request = c.Request.WithContext(telemetry.WithTraceID(c.Request.Context(), traceID))

		// Set response headers for client tracking
		c.Writer.Header().Set(HeaderAirouteTraceID, traceID)
		c.Writer.Header().Set(HeaderRequestID, traceID)

		c.Next()
	}
}

// extractTraceID inspects incoming headers (X-Airoute-Trace-ID, X-Request-ID, X-Trace-ID, W3C traceparent).
func extractTraceID(c *gin.Context) string {
	if tid := strings.TrimSpace(c.GetHeader(HeaderAirouteTraceID)); tid != "" {
		return tid
	}
	if tid := strings.TrimSpace(c.GetHeader(HeaderRequestID)); tid != "" {
		return tid
	}
	if tid := strings.TrimSpace(c.GetHeader(HeaderTraceID)); tid != "" {
		return tid
	}
	if tp := strings.TrimSpace(c.GetHeader(HeaderTraceParent)); tp != "" {
		// W3C format: version-trace_id-parent_id-trace_flags (e.g., 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01)
		parts := strings.Split(tp, "-")
		if len(parts) >= 4 && len(parts[1]) == 32 {
			return "tr-" + parts[1]
		}
	}
	return ""
}

// GetTraceID extracts the TraceID from a Gin context, with a fallback generator.
func GetTraceID(c *gin.Context) string {
	if c == nil {
		return GenerateTraceID()
	}
	if val, exists := c.Get(ContextKeyTraceID); exists {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	tid := extractTraceID(c)
	if tid != "" {
		return tid
	}
	return GenerateTraceID()
}

// GetTraceIDFromContext retrieves the TraceID from a Go standard context.Context.
func GetTraceIDFromContext(ctx context.Context) string {
	return telemetry.TraceIDFromContext(ctx)
}

// WithTraceID attaches traceID to a standard context. Callers outside this package
// (tests, background jobs) use it instead of hand-building a context key.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return telemetry.WithTraceID(ctx, traceID)
}

func sanitizeURLQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	for i, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			k := strings.ToLower(kv[0])
			// sessionId is a credential of the MCP SSE transport: the POST URL
			// carries it on every message, so an unredacted access log let any
			// log reader take over another caller's session (A08). api_key is the
			// snake_case spelling of the same credential.
			if k == "key" || k == "apikey" || k == "api_key" || k == "token" ||
				k == "secret" || k == "password" || k == "sessionid" || k == "session_id" {
				parts[i] = kv[0] + "=[REDACTED]"
			}
		}
	}
	return strings.Join(parts, "&")
}

// AccessLogMiddleware outputs structured JSON logs for all processed requests,
// embedding the TraceID for centralized log aggregation and monitoring.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		// Skip high-frequency probes and metrics scrapers to keep logs clean
		if path == "/health" || path == "/api/v1/public/status" || path == "/metrics" {
			return
		}

		traceID := GetTraceID(c)
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := ResolveClientIP(c)

		if raw != "" {
			path = path + "?" + sanitizeURLQuery(raw)
		}

		// Identity and failure reason are part of the access line on purpose.
		// Without them an operator staring at "status=502" cannot tell which
		// tenant/key produced it, nor what the gateway actually reported, and
		// the only way to find out is a database join against usage_logs.
		fields := []any{
			"trace_id", traceID,
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"ip", clientIP,
			"bytes", c.Writer.Size(),
			"tenant", c.GetString(ContextKeyTenant),
			"key", accessLogKey(c),
		}
		if status >= 400 {
			if message := c.GetString(ContextKeyErrorMessage); message != "" {
				fields = append(fields, "error", message)
			}
		} else if auditStatus := c.GetInt(ContextKeyAuditStatus); auditStatus != 0 && auditStatus != status {
			// The wire and the audit trail disagree: the response head was already
			// a 200 when the gateway recorded a 499 because the downstream hung up.
			// Printing only "status=200" hid the abort from stdout and forced a
			// database join to discover that the request had failed AND been billed.
			fields = append(fields, "audit_status", auditStatus)
			if message := c.GetString(ContextKeyErrorMessage); message != "" {
				fields = append(fields, "error", message)
			}
		}
		telemetry.ScopedLogger("system").Info("http_access", fields...)
	}
}

// accessLogKey renders the authenticated key for the access line.
//
// ContextKeyAPIKey holds the stored digest (never the credential) and
// ContextKeyAPIKeyID holds the public identifier; the identifier is preferred so
// the line stays comparable with the console's key column.
func accessLogKey(c *gin.Context) string {
	if keyID := c.GetString(ContextKeyAPIKeyID); keyID != "" {
		return keyID
	}
	return maskAPIKey(c.GetString(ContextKeyAPIKey))
}

// maskAPIKey keeps enough of a value to tell two entries apart while never
// writing a usable credential to stdout or the log archive.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return key[:2] + "***"
	}
	return key[:10] + "***"
}
