package storage

import (
	"strings"
	"testing"
	"time"
)

func TestTruncateSnippetRedactsAndClamps(t *testing.T) {
	payload := `{"model":"gpt","api_key":"sk-secret-value","messages":[]}`
	got := TruncateSnippet(payload, 4096)
	if strings.Contains(got, "sk-secret-value") {
		t.Fatalf("expected credential to be redacted, got %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", got)
	}

	long := strings.Repeat("a", 10000)
	clamped := TruncateSnippet(long, 100)
	if len(clamped) > 200 {
		t.Fatalf("expected clamped snippet, got %d bytes", len(clamped))
	}
	if !strings.Contains(clamped, "truncated") {
		t.Fatalf("expected truncation marker, got %q", clamped[:40])
	}
}

func TestTraceCapturePolicy(t *testing.T) {
	errorsOnly := &TraceCapture{mode: "errors", sample: 0}
	if !errorsOnly.Enabled(500) {
		t.Fatal("error responses must be captured in errors mode")
	}
	if errorsOnly.Enabled(200) {
		t.Fatal("successful responses must not be captured in errors mode")
	}

	off := &TraceCapture{mode: "off"}
	if off.Enabled(500) {
		t.Fatal("off mode must never capture")
	}

	all := &TraceCapture{mode: "all", sample: 100}
	if !all.Enabled(200) {
		t.Fatal("all mode with 100% sample must capture successes")
	}
}

func TestRequestTraceRoundTrip(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	rec := &RequestTraceRecord{
		TraceID:         "tr-test-1",
		ChatID:          "chat-1",
		SessionID:       "sess-1",
		APIKey:          "sk-trace",
		Model:           "m",
		Channel:         "c",
		StatusCode:      502,
		ErrorMessage:    "boom",
		RequestSnippet:  `{"model":"m"}`,
		ResponseSnippet: `{"error":"boom"}`,
		DurationMs:      12,
	}
	if err := repo.RecordRequestTrace(rec); err != nil {
		t.Fatalf("record trace: %v", err)
	}

	got, err := repo.ListRequestTraces(TraceFilter{TraceID: "tr-test-1", Limit: 10})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	if len(got) != 1 || got[0].StatusCode != 502 || got[0].Model != "m" {
		t.Fatalf("unexpected trace rows: %+v", got)
	}

	// TTL purge removes rows older than the cutoff.
	if _, err := repo.PurgeRequestTraces(time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("purge: %v", err)
	}
	got, _ = repo.ListRequestTraces(TraceFilter{TraceID: "tr-test-1", Limit: 10})
	if len(got) != 0 {
		t.Fatalf("expected purged traces, got %d", len(got))
	}
}
