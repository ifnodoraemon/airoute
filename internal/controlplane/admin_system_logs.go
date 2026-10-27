package controlplane

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// GetSystemLogs returns recent in-memory runtime system logs for troubleshooting.
func (h *AdminHandler) GetSystemLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 100
	} else if limit > 1000 {
		limit = 1000
	}

	level := c.Query("level")
	q := c.Query("q")
	category := c.Query("category")
	traceID := c.Query("trace_id")

	rb := telemetry.GlobalRingBuffer
	if rb == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"data":    []*telemetry.SystemLogEntry{},
			"total":   0,
			"node_id": "unknown",
		})
		return
	}

	entries := rb.GetRecentLogs(limit, level, q, category, traceID)
	nodeID := "airoute-gateway"
	if len(entries) > 0 && entries[0].NodeID != "" {
		nodeID = entries[0].NodeID
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    entries,
		"total":   len(entries),
		"node_id": nodeID,
	})
}

// StreamSystemLogs streams live system logs via Server-Sent Events (SSE).
func (h *AdminHandler) StreamSystemLogs(c *gin.Context) {
	rb := telemetry.GlobalRingBuffer
	if rb == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "系统日志流服务未就绪"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // Disable Nginx proxy buffering

	// The live log stream is meant to stay open for as long as the operator keeps
	// the console page open. http.Server.WriteTimeout is an absolute deadline for
	// the whole response, so without clearing it the stream dies at 600s with the
	// shipped Compose config (180s with the Helm chart). Mirror InitSSEStream.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})

	c.Writer.Flush()

	ch, cancel := rb.Subscribe()
	defer cancel()

	// Initial handshake event
	fmt.Fprintf(c.Writer, "event: connected\ndata: {\"status\":\"connected\",\"time\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
	c.Writer.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-heartbeat.C:
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			return true
		case entry, ok := <-ch:
			if !ok {
				return false
			}
			bytes, err := json.Marshal(entry)
			if err == nil {
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(bytes))
			}
			return true
		}
	})
}
