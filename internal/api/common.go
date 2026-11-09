package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// writeStreamUsageTrailer appends the billing metadata as an SSE comment so
// streaming clients still receive it after the response head was flushed.
// SSE comment lines start with ':' and are ignored by every compliant client.
func writeStreamUsageTrailer(w io.Writer, flusher http.Flusher, cost, savedCost float64, cachedTokens int) {
	if w == nil {
		return
	}
	line := fmt.Sprintf(": airoute-cost=%.6f saved-cost=%.6f cached-tokens=%d\n\n", cost, savedCost, cachedTokens)
	if _, err := io.WriteString(w, line); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// RespondOpenAIError returns a standardized OpenAI API error JSON response.
func RespondOpenAIError(c *gin.Context, statusCode int, message, errType, code string) {
	if errType == "" {
		errType = "invalid_request_error"
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"code":    code,
		},
	})
}

// RespondAnthropicError returns a standardized Anthropic API error JSON response.
func RespondAnthropicError(c *gin.Context, statusCode int, message, errType string) {
	if errType == "" {
		errType = "invalid_request_error"
	}
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// RespondGeminiError returns a standardized Google Gemini API error JSON response.
func RespondGeminiError(c *gin.Context, statusCode int, message, status string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    statusCode,
			"message": message,
			"status":  status,
		},
	})
}

// InitSSEStream configures standard Server-Sent Events headers and flushes the writer.
func InitSSEStream(c *gin.Context) (http.Flusher, bool) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// Remove WriteTimeout deadline for SSE streaming so long-running responses
	// (> 180s) won't be terminated mid-stream by http.Server.WriteTimeout.
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetWriteDeadline(time.Time{})

	flusher, ok := c.Writer.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return flusher, ok
}
