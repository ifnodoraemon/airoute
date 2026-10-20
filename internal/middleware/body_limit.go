package middleware

import (
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

// defaultMaxBodyBytes caps request bodies at 64 MiB, comfortably above the
// largest OpenAI-compatible uploads (25 MiB audio) while preventing trivial
// memory-exhaustion attacks.
const defaultMaxBodyBytes int64 = 64 << 20

// ResolveMaxBodyBytes returns the configured body limit, falling back to the
// default when GATEWAY_MAX_BODY_BYTES is unset or invalid.
func ResolveMaxBodyBytes() int64 {
	if v := os.Getenv("GATEWAY_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxBodyBytes
}

// BodyLimit rejects request bodies larger than maxBytes. Handlers see the
// standard "http: request body too large" error from the reader.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
