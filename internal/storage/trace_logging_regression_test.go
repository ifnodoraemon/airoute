package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Regression for the Redis-stream fallback warning (ST-08).
//
// The hot path called a plain logger once per usage record, so a Redis outage
// produced one WARN per request. The throttled helper must emit at most one line
// per window for a burst.
func TestUsageLogXAddFallbackWarnIsThrottled(t *testing.T) {
	telemetry.ResetWarnGate()
	t.Cleanup(telemetry.ResetWarnGate)

	for i := 0; i < 20; i++ {
		warnUsageLogXAddFallback(errors.New("redis: connection refused"))
	}

	count := 0
	for _, entry := range telemetry.GlobalRingBuffer.GetRecentLogs(200, "", "", "", "") {
		if entry.Message == "failed to push log to Redis Stream, falling back to local memory buffer" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("burst of 20 XADD failures produced %d log lines, want 1 (throttled)", count)
	}
}

// Regression for trace capture logging (ST-07).
//
// The capture worker logged through slog.Default(), which writes non-JSON text to
// stderr and never reaches the console ring buffer, so "failed to persist request
// trace" was invisible in the UI. It must now go through the unified logger.
func TestTraceCapturePersistFailureReachesUnifiedLogger(t *testing.T) {
	telemetry.ResetWarnGate()
	t.Cleanup(telemetry.ResetWarnGate)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo := NewRepository(db)
	// Force the persistence path to fail: the repository outlives the database it
	// was opened on.
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	tc := &TraceCapture{
		mode:   "errors",
		sample: 100,
		queue:  make(chan *RequestTraceRecord, 4),
		stop:   make(chan struct{}),
		repo:   repo,
	}
	tc.queue <- &RequestTraceRecord{TraceID: "tr-probe-1", StatusCode: 500}
	done := make(chan struct{})
	// worker() calls wg.Done(), so the test must account for it.
	tc.wg.Add(1)
	go func() {
		defer close(done)
		tc.worker()
	}()
	// worker blocks on the next receive; wait for the log entry to appear.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if findRingBufferMessage("failed to persist request trace") {
			close(tc.stop)
			<-done
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(tc.stop)
	<-done
	t.Fatal("trace persistence failure was not routed through the unified JSON logger/ring buffer")
}

func findRingBufferMessage(want string) bool {
	for _, entry := range telemetry.GlobalRingBuffer.GetRecentLogs(500, "", "", "", "") {
		if entry.Message == want && entry.Category == "system" {
			return true
		}
	}
	return false
}
