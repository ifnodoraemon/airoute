package storage

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// TraceCapture archives redacted request/response snippets for troubleshooting.
// It is intentionally off the hot path: writes go through a bounded queue and a
// single background worker.
type TraceCapture struct {
	mu      sync.RWMutex
	mode    string // "off" | "errors" | "all"
	sample  float64
	ttl     time.Duration
	maxSnip int
	queue   chan *RequestTraceRecord
	stop    chan struct{}
	// stopOnce makes Stop idempotent now that two goroutines may be running
	// (capture worker and retention purger).
	stopOnce sync.Once
	wg       sync.WaitGroup
	repo     *Repository
	// purgeInterval is how often expired snapshots are deleted. It is a field
	// (not a constant) so tests can drive the retention loop without waiting an
	// hour; production always uses one hour.
	purgeInterval time.Duration
	dropped       int64
	recorded      int64
}

// GlobalTraceCapture is the process-wide trace archiver.
var GlobalTraceCapture *TraceCapture

// InitTraceCapture starts the archive worker according to environment settings.
//
// Retention is deliberately independent of the capture mode: switching capture
// off stops new snapshots from being written, but the rows already archived must
// still expire, so the purge loop runs whenever a repository is available.
func InitTraceCapture(repo *Repository) *TraceCapture {
	cfg := ResolveTraceConfig()
	tc := &TraceCapture{
		mode:          cfg.mode,
		sample:        cfg.sample,
		ttl:           cfg.ttl,
		maxSnip:       cfg.maxSnip,
		queue:         make(chan *RequestTraceRecord, 1024),
		stop:          make(chan struct{}),
		repo:          repo,
		purgeInterval: time.Hour,
	}
	GlobalTraceCapture = tc
	tc.start()
	return tc
}

// start launches the background goroutines this instance needs.
//
// It is separate from InitTraceCapture so a test can build a TraceCapture with a
// short purge interval and exercise the same wiring the server uses.
func (tc *TraceCapture) start() {
	if tc == nil || tc.repo == nil {
		return
	}
	tc.wg.Add(1)
	go tc.purgeWorker()
	if tc.mode == "off" {
		return
	}
	tc.wg.Add(1)
	go tc.worker()
}

type resolvedTraceConfig struct {
	mode    string
	sample  float64
	ttl     time.Duration
	maxSnip int
}

func ResolveTraceConfig() resolvedTraceConfig {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_TRACE_CAPTURE")))
	switch mode {
	case "all", "errors", "off":
	default:
		mode = "errors"
	}
	sample := 100.0
	if v := os.Getenv("GATEWAY_TRACE_SAMPLE"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			if n < 0 {
				n = 0
			}
			if n > 100 {
				n = 100
			}
			sample = n
		}
	}
	ttl := 72 * time.Hour
	if v := os.Getenv("GATEWAY_TRACE_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttl = time.Duration(n) * time.Hour
		}
	}
	maxSnip := 4096
	if v := os.Getenv("GATEWAY_TRACE_MAX_SNIPPET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxSnip = n
		}
	}
	return resolvedTraceConfig{mode: mode, sample: sample, ttl: ttl, maxSnip: maxSnip}
}

// Enabled reports whether a trace with the given status code is eligible for
// capture.
//
// It deliberately does not apply the success sampling rate: Record is the single
// place that draws the sample, so a caller may use Enabled as a cheap pre-check
// (for example to skip marshalling a response) without consuming an independent
// random draw. Evaluating the rate in both places compounded it — three checks
// on the success path turned a configured 10% into roughly 0.1%.
func (tc *TraceCapture) Enabled(statusCode int) bool {
	if tc == nil || tc.mode == "off" {
		return false
	}
	if statusCode >= 400 {
		return true
	}
	return tc.mode == "all"
}

// Record enqueues a trace snapshot without blocking the request path.
//
// This is the single sampling decision point: it returns true only when the
// record was actually queued. A false return means the capture policy rejected
// this request or the queue was full, so callers can treat it as "not archived".
func (tc *TraceCapture) Record(rec *RequestTraceRecord) bool {
	if tc == nil || rec == nil || tc.mode == "off" {
		return false
	}
	if !tc.Enabled(rec.StatusCode) {
		return false
	}
	// Successful requests are sampled; errors are always archived.
	if rec.StatusCode < 400 && tc.sample < 100 && rand.Float64()*100 >= tc.sample {
		return false
	}
	rec.RequestSnippet = TruncateSnippet(rec.RequestSnippet, tc.maxSnip)
	rec.ResponseSnippet = TruncateSnippet(rec.ResponseSnippet, tc.maxSnip)
	rec.ErrorMessage = TruncateSnippet(rec.ErrorMessage, 1024)
	select {
	case tc.queue <- rec:
		tc.mu.Lock()
		tc.recorded++
		tc.mu.Unlock()
		return true
	default:
		tc.mu.Lock()
		tc.dropped++
		tc.mu.Unlock()
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "trace-capture-queue-full", 30*time.Second,
			"trace capture queue full, dropping snapshots", "trace_id", rec.TraceID)
		return false
	}
}

func (tc *TraceCapture) worker() {
	defer tc.wg.Done()

	for {
		select {
		case <-tc.stop:
			tc.drain()
			return
		case rec := <-tc.queue:
			if err := tc.repo.RecordRequestTrace(rec); err != nil {
				telemetry.WarnLogger(telemetry.ScopedLogger("system"), "trace-capture-persist-failed", 30*time.Second,
					"failed to persist request trace", "error", err.Error(), "trace_id", rec.TraceID)
			}
		}
	}
}

// purgeWorker deletes snapshots past the retention window on its own schedule.
//
// It runs even when capture is "off": the TTL is a property of the rows that are
// already stored, not of whether new ones are being written.
func (tc *TraceCapture) purgeWorker() {
	defer tc.wg.Done()
	interval := tc.purgeInterval
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// One pass at startup so a backlog left by a prior process is reclaimed as
	// soon as possible rather than an hour later.
	tc.purgeOnce()
	for {
		select {
		case <-tc.stop:
			return
		case <-ticker.C:
			tc.purgeOnce()
		}
	}
}

func (tc *TraceCapture) purgeOnce() {
	if tc == nil || tc.repo == nil || tc.ttl <= 0 {
		return
	}
	if n, err := tc.repo.PurgeRequestTraces(time.Now().Add(-tc.ttl)); err == nil && n > 0 {
		telemetry.ScopedLogger("system").Info("purged expired request traces", "count", n)
	}
}

func (tc *TraceCapture) drain() {
	for {
		select {
		case rec := <-tc.queue:
			_ = tc.repo.RecordRequestTrace(rec)
		default:
			return
		}
	}
}

// Stop flushes queued traces and terminates the background goroutines.
func (tc *TraceCapture) Stop() {
	if tc == nil {
		return
	}
	tc.stopOnce.Do(func() {
		if tc.stop != nil {
			close(tc.stop)
		}
	})
	tc.wg.Wait()
}
