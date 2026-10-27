package controlplane

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// The live audit-log stream is a long-lived SSE response, so it must survive
// http.Server.WriteTimeout (600s Compose / 180s Helm). This test gives the test
// server a 150ms write timeout, then proves a log line written well after that
// deadline still reaches the client.
func TestSystemLogStreamOutlivesServerWriteTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := &AdminHandler{}
	engine.GET("/api/v1/admin/system/logs/stream", handler.StreamSystemLogs)

	srv := httptest.NewUnstartedServer(engine)
	srv.Config.WriteTimeout = 150 * time.Millisecond
	srv.Start()
	defer srv.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(srv.URL + "/api/v1/admin/system/logs/stream")
	if err != nil {
		t.Fatalf("connect to log stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	if _, err := readSSEEvent(reader, "connected"); err != nil {
		t.Fatalf("read connected event: %v", err)
	}

	// Cross the server write timeout while the stream is open.
	time.Sleep(400 * time.Millisecond)
	telemetry.GlobalRingBuffer.Append(&telemetry.SystemLogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "deploy-audit stream deadline probe",
		Category:  "system",
	})

	entry, err := readSSEEvent(reader, "log")
	if err != nil {
		t.Fatalf("read log event after server WriteTimeout: %v", err)
	}
	if !strings.Contains(entry, "deploy-audit stream deadline probe") {
		t.Fatalf("unexpected log payload %q", entry)
	}
}

func readSSEEvent(r *bufio.Reader, wantEvent string) (string, error) {
	event := ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("waiting for event %q: %w", wantEvent, err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if event == wantEvent {
				return strings.TrimPrefix(line, "data: "), nil
			}
		}
	}
}
