package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func TestSystemLogsAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Populate telemetry ring buffer
	telemetry.GlobalRingBuffer.Append(&telemetry.SystemLogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "cluster redis sync ready",
		Category:  "sync",
		TraceID:   "tr-test-123",
	})
	telemetry.GlobalRingBuffer.Append(&telemetry.SystemLogEntry{
		Timestamp: time.Now(),
		Level:     "ERROR",
		Message:   "circuit breaker tripped for channel gpustack",
		Category:  "breaker",
		TraceID:   "tr-test-456",
	})

	handler := &AdminHandler{}

	// 1. Get recent logs
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/admin/system/logs?limit=10", nil)

		handler.GetSystemLogs(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Code int                         `json:"code"`
			Data []*telemetry.SystemLogEntry `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(resp.Data) < 2 {
			t.Fatalf("expected at least 2 entries, got %d", len(resp.Data))
		}
	}

	// 2. Filter by level
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/admin/system/logs?level=ERROR", nil)

		handler.GetSystemLogs(c)

		var resp struct {
			Code int                         `json:"code"`
			Data []*telemetry.SystemLogEntry `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		for _, e := range resp.Data {
			if e.Level != "ERROR" {
				t.Fatalf("expected only ERROR, got %s", e.Level)
			}
		}
	}

	// 3. Filter by trace_id
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/admin/system/logs?trace_id=tr-test-123", nil)

		handler.GetSystemLogs(c)

		var resp struct {
			Code int                         `json:"code"`
			Data []*telemetry.SystemLogEntry `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp.Data) == 0 || resp.Data[0].TraceID != "tr-test-123" {
			t.Fatalf("expected matching trace_id, got %+v", resp.Data)
		}
	}
}
