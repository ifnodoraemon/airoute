package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// newTraceTestCapture builds an in-memory repository with a trace archiver whose
// policy comes from the environment, so the test exercises the same resolution the
// server uses.
func newTraceTestCapture(t *testing.T) (*storage.Repository, *storage.TraceCapture) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	tc := storage.InitTraceCapture(repo)
	t.Cleanup(func() {
		tc.Stop()
		storage.GlobalTraceCapture = nil
	})
	return repo, tc
}

func traceTestContext(snapshot string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(""))
	if snapshot != "" {
		c.Set(ContextKeyRequestSnapshot, snapshot)
	}
	return c
}

func waitForTraces(t *testing.T, repo *storage.Repository, want int, timeout time.Duration) []*storage.RequestTraceRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		rows, err := repo.ListRequestTraces(storage.TraceFilter{Limit: 10})
		if err != nil {
			t.Fatalf("list traces: %v", err)
		}
		if len(rows) >= want || time.Now().After(deadline) {
			return rows
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Mode "all" is documented to sample *successful* requests, but only the 499 and
// error paths ever archived a trace, so the knob captured nothing. A successful
// unary response must now leave one redacted row.
func TestRecordSuccessTraceCapturesSampledSuccess(t *testing.T) {
	t.Setenv("GATEWAY_TRACE_CAPTURE", "all")
	t.Setenv("GATEWAY_TRACE_SAMPLE", "100")
	repo, _ := newTraceTestCapture(t)

	const secret = "sk-live-should-never-be-archived-1234567890"
	c := traceTestContext(`{"model":"mock-chat","api_key":"` + secret + `","messages":[{"role":"user","content":"hi"}]}`)
	RecordSuccessTrace(c, "mock-chat", "mock-a", 12*time.Millisecond, gin.H{"ok": true})

	rows := waitForTraces(t, repo, 1, 3*time.Second)
	if len(rows) != 1 {
		t.Fatalf("expected one archived success trace, got %d", len(rows))
	}
	row := rows[0]
	if row.StatusCode != 200 || row.Model != "mock-chat" || row.Channel != "mock-a" {
		t.Fatalf("unexpected trace row: %+v", row)
	}
	if strings.Contains(row.RequestSnippet, secret) {
		t.Errorf("request snippet was not redacted: %q", row.RequestSnippet)
	}
	if !strings.Contains(row.RequestSnippet, "[REDACTED]") {
		t.Errorf("expected redaction marker in request snippet: %q", row.RequestSnippet)
	}
	if row.ResponseSnippet == "" {
		t.Errorf("expected the response snippet to be archived")
	}
}

// The default "errors" mode must not archive successful requests: the new call
// site has to be a no-op there.
func TestRecordSuccessTraceRespectsErrorsMode(t *testing.T) {
	t.Setenv("GATEWAY_TRACE_CAPTURE", "errors")
	repo, _ := newTraceTestCapture(t)

	c := traceTestContext(`{"model":"mock-chat","messages":[]}`)
	RecordSuccessTrace(c, "mock-chat", "mock-a", time.Millisecond, gin.H{"ok": true})

	if rows := waitForTraces(t, repo, 1, 250*time.Millisecond); len(rows) != 0 {
		t.Fatalf("errors mode must not archive successes, got %d rows", len(rows))
	}
}
