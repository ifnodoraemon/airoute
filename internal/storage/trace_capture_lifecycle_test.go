package storage

// Regression tests for JOBS-04 (2026-10-29 bughunt round 3): after Stop() the
// trace archiver no longer has a worker, so Record() must report false instead of
// claiming the snapshot was queued for archiving.

import (
	"testing"
)

func TestTraceCaptureRejectsRecordsAfterStop(t *testing.T) {
	t.Setenv("GATEWAY_TRACE_CAPTURE", "all")
	t.Setenv("GATEWAY_TRACE_SAMPLE", "100")

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	prev := GlobalTraceCapture
	defer func() { GlobalTraceCapture = prev }()

	tc := InitTraceCapture(repo)
	tc.Stop()

	if queued := tc.Record(&RequestTraceRecord{TraceID: "tr-after-stop", ChatID: "chat", StatusCode: 500}); queued {
		t.Fatal("Record after Stop must report false: there is no worker left to archive the snapshot")
	}
	if tc.Enabled(500) {
		t.Fatal("Enabled after Stop must report false so callers skip building snippets")
	}
	var rows int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM request_traces WHERE trace_id = 'tr-after-stop'`).Scan(&rows); err != nil {
		t.Fatalf("count traces: %v", err)
	}
	if rows != 0 {
		t.Fatalf("no trace row may be written after Stop, got %d", rows)
	}
}

func TestTraceCaptureStopIsIdempotent(t *testing.T) {
	t.Setenv("GATEWAY_TRACE_CAPTURE", "errors")

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	prev := GlobalTraceCapture
	defer func() { GlobalTraceCapture = prev }()

	tc := InitTraceCapture(repo)
	tc.Stop()
	tc.Stop() // must not panic or block
}

// A trace accepted before Stop must still be persisted: the rejection applies to
// records arriving after shutdown, not to the ones already queued.
func TestTraceCaptureDrainsQueuedRecordOnStop(t *testing.T) {
	t.Setenv("GATEWAY_TRACE_CAPTURE", "all")
	t.Setenv("GATEWAY_TRACE_SAMPLE", "100")

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	prev := GlobalTraceCapture
	defer func() { GlobalTraceCapture = prev }()

	tc := InitTraceCapture(repo)
	if queued := tc.Record(&RequestTraceRecord{TraceID: "tr-before-stop", StatusCode: 500}); !queued {
		t.Fatal("a record accepted before Stop must be queued")
	}
	tc.Stop()

	var rows int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM request_traces WHERE trace_id = 'tr-before-stop'`).Scan(&rows); err != nil {
		t.Fatalf("count traces: %v", err)
	}
	if rows != 1 {
		t.Fatalf("the queued snapshot must be drained by Stop, got %d rows", rows)
	}
}
