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
	ContextKeyTraceID    = "trace_id"
	HeaderAirouteTraceID = "X-Airoute-Trace-ID"
	HeaderRequestID      = "X-Request-ID"
	HeaderTraceID        = "X-Trace-ID"
	HeaderTraceParent    = "traceparent"
)

// GenerateTraceID creates a unique, high-performance distributed trace identifier.
func GenerateTraceID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("tr-%x-%s", time.Now().UnixNano()%100000000, hex.EncodeToString(b))
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
		ctx := context.WithValue(c.Request.Context(), ContextKeyTraceID, traceID)
		c.Request = c.Request.WithContext(ctx)

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
	if ctx == nil {
		return ""
	}
	if val := ctx.Value(ContextKeyTraceID); val != nil {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// AccessLogMiddleware outputs structured JSON logs for all processed requests,
// embedding the TraceID for centralized log aggregation and monitoring.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		// Skip health checks to keep logs clean
		if path == "/health" || path == "/api/v1/public/status" {
			return
		}

		traceID := GetTraceID(c)
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()

		if raw != "" {
			path = path + "?" + raw
		}

		telemetry.Logger.Info("http_access",
			"trace_id", traceID,
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"ip", clientIP,
			"bytes", c.Writer.Size(),
		)
	}
}
