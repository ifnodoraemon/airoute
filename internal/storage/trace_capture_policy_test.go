package storage

import (
	"testing"
	"time"
)

// E-01 regression: the success sampling rate is drawn exactly once per record.
//
// Before the fix Enabled(), RecordTrace() and Record() each evaluated the rate,
// so a configured 10% produced roughly 0.1% of the rows. The band below is a
// statistical guard (mean 100, sigma ~9.5 over 1000 draws): anything outside it
// means the decision is still compounding or being skipped.
func TestTraceRecordIsTheSingleSamplingPoint(t *testing.T) {
	const draws = 1000
	tc := &TraceCapture{
		mode:   "all",
		sample: 10,
		queue:  make(chan *RequestTraceRecord, draws+1),
		stop:   make(chan struct{}),
	}

	queued := 0
	for i := 0; i < draws; i++ {
		if tc.Record(&RequestTraceRecord{TraceID: "tr-sample", StatusCode: 200}) {
			queued++
		}
	}
	if queued < 50 || queued > 200 {
		t.Fatalf("Record archived %d/%d successful traces at sample=10, want 50..200", queued, draws)
	}
	if int(tc.recorded) != queued {
		t.Fatalf("recorded counter = %d, want %d", tc.recorded, queued)
	}

	// Enabled() must not consume a sample: it reports policy eligibility only, so
	// callers can use it to skip marshalling a response.
	for i := 0; i < draws; i++ {
		if !tc.Enabled(200) {
			t.Fatal("Enabled(200) must stay true in all mode regardless of the sample rate")
		}
	}
	if tc.Enabled(500) != true {
		t.Fatal("errors must remain eligible regardless of the sample rate")
	}
	if (&TraceCapture{mode: "errors", sample: 0}).Enabled(200) {
		t.Fatal("errors mode must not report successes as eligible")
	}
}

// E-01: Record reports whether the snapshot was actually queued.
func TestTraceRecordReportsQueueOutcome(t *testing.T) {
	off := &TraceCapture{mode: "off", sample: 100, queue: make(chan *RequestTraceRecord, 4), stop: make(chan struct{})}
	if off.Record(&RequestTraceRecord{TraceID: "tr-off", StatusCode: 500}) {
		t.Fatal("capture=off must report false")
	}
	if off.recorded != 0 {
		t.Fatalf("capture=off counted %d archived rows", off.recorded)
	}

	tc := &TraceCapture{mode: "errors", sample: 100, queue: make(chan *RequestTraceRecord, 1), stop: make(chan struct{})}
	if !tc.Record(&RequestTraceRecord{TraceID: "tr-first", StatusCode: 500}) {
		t.Fatal("the first record must be queued")
	}
	if tc.Record(&RequestTraceRecord{TraceID: "tr-full", StatusCode: 500}) {
		t.Fatal("a full queue must report false")
	}
	if tc.dropped != 1 {
		t.Fatalf("dropped counter = %d, want 1", tc.dropped)
	}
}

// E-02 regression: retention must run even when capture is switched off.
//
// The purge used to live inside the capture worker, which is not started when
// mode is "off", so archived rows lived forever after an operator disabled capture.
func TestTraceRetentionRunsWithCaptureOff(t *testing.T) {
	t.Setenv("GATEWAY_TRACE_CAPTURE", "off")
	t.Setenv("GATEWAY_TRACE_TTL_HOURS", "1")
	previous := GlobalTraceCapture
	t.Cleanup(func() { GlobalTraceCapture = previous })

	db, err := OpenDB(t.TempDir() + "/trace-retention.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	for _, traceID := range []string{"tr-expired", "tr-fresh"} {
		if err := repo.RecordRequestTrace(&RequestTraceRecord{
			TraceID: traceID, ChatID: "chat", StatusCode: 500, ErrorMessage: "boom",
		}); err != nil {
			t.Fatalf("record trace %s: %v", traceID, err)
		}
	}
	// Backdate one row past the one-hour TTL.
	if _, err := db.Exec(`UPDATE request_traces SET created_at = ? WHERE trace_id = ?`,
		time.Now().Add(-2*time.Hour).UTC().Format(sqliteTimestampLayout), "tr-expired"); err != nil {
		t.Fatalf("backdate trace: %v", err)
	}

	tc := InitTraceCapture(repo)
	t.Cleanup(tc.Stop)

	deadline := time.Now().Add(5 * time.Second)
	for {
		rows, err := repo.ListRequestTraces(TraceFilter{Limit: 10})
		if err != nil {
			t.Fatalf("list traces: %v", err)
		}
		expired, fresh := false, false
		for _, row := range rows {
			switch row.TraceID {
			case "tr-expired":
				expired = true
			case "tr-fresh":
				fresh = true
			}
		}
		if !expired {
			if !fresh {
				t.Fatal("retention deleted the fresh trace too")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired trace was not purged while capture was off")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Capture itself must stay off.
	if tc.Record(&RequestTraceRecord{TraceID: "tr-late", StatusCode: 500}) {
		t.Fatal("capture=off still queued a record")
	}
}
