package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestCaptureRequestSnapshot_LargeBodyNotTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Ensure trace capture is non-nil so CaptureRequestSnapshot actually executes
	storage.GlobalTraceCapture = &storage.TraceCapture{}

	// Construct a JSON payload significantly larger than snapshotMaxBytes (32KB), e.g. 100KB
	largePrompt := strings.Repeat("A", 100*1024)
	largePayload := map[string]any{
		"model": "claude-3-7-sonnet",
		"messages": []map[string]any{
			{"role": "user", "content": largePrompt},
		},
		"max_tokens": 100,
	}
	bodyBytes, err := json.Marshal(largePayload)
	if err != nil {
		t.Fatalf("failed to marshal large payload: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(bodyBytes))

	// Execute CaptureRequestSnapshot
	CaptureRequestSnapshot(c)

	// Verify the snapshot in context is capped at snapshotMaxBytes
	snapVal, exists := c.Get(ContextKeyRequestSnapshot)
	if !exists {
		t.Fatal("expected request_snapshot in context")
	}
	snapStr := snapVal.(string)
	if len(snapStr) > snapshotMaxBytes {
		t.Fatalf("expected snapshot capped at %d bytes, got %d", snapshotMaxBytes, len(snapStr))
	}

	// Verify c.Request.Body is NOT truncated and can be read in full
	fullBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("failed to read full body from c.Request.Body: %v", err)
	}
	if len(fullBody) != len(bodyBytes) {
		t.Fatalf("expected c.Request.Body to have %d bytes, got %d (was truncated!)", len(bodyBytes), len(fullBody))
	}

	// Verify JSON parsing succeeds without unexpected EOF
	c.Request.Body = io.NopCloser(bytes.NewReader(fullBody))
	var req AnthropicInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		t.Fatalf("c.ShouldBindJSON failed with: %v (expected successful unmarshal of large request)", err)
	}
	if req.Model != "claude-3-7-sonnet" {
		t.Fatalf("expected model 'claude-3-7-sonnet', got %s", req.Model)
	}
}
