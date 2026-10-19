package storage

import (
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TraceCapture archives redacted request/response snippets for troubleshooting.
// It is intentionally off the hot path: writes go through a bounded queue and a
// single background worker.
type TraceCapture struct {
	mu       sync.RWMutex
	mode     string // "off" | "errors" | "all"
	sample   float64
	ttl      time.Duration
	maxSnip  int
	queue    chan *RequestTraceRecord
	stop     chan struct{}
	wg       sync.WaitGroup
	repo     *Repository
	dropped  int64
	recorded int64
}

// GlobalTraceCapture is the process-wide trace archiver.
var GlobalTraceCapture *TraceCapture

// InitTraceCapture starts the archive worker according to environment settings.
func InitTraceCapture(repo *Repository) *TraceCapture {
	cfg := ResolveTraceConfig()
	tc := &TraceCapture{
		mode:    cfg.mode,
		sample:  cfg.sample,
		ttl:     cfg.ttl,
		maxSnip: cfg.maxSnip,
		queue:   make(chan *RequestTraceRecord, 1024),
		stop:    make(chan struct{}),
		repo:    repo,
	}
	GlobalTraceCapture = tc
	if tc.mode == "off" || repo == nil {
		return tc
	}
	tc.wg.Add(1)
	go tc.worker()
	return tc
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

// Enabled reports whether a trace with the given status code should be archived.
func (tc *TraceCapture) Enabled(statusCode int) bool {
	if tc == nil || tc.mode == "off" {
		return false
	}
	if statusCode >= 400 {
		return true
	}
	if tc.mode != "all" {
		return false
	}
	if tc.sample >= 100 {
		return true
	}
	return rand.Float64()*100 < tc.sample
}

// Record enqueues a trace snapshot without blocking the request path.
func (tc *TraceCapture) Record(rec *RequestTraceRecord) {
	if tc == nil || rec == nil || tc.mode == "off" {
		return
	}
	if !tc.Enabled(rec.StatusCode) {
		return
	}
	rec.RequestSnippet = TruncateSnippet(rec.RequestSnippet, tc.maxSnip)
	rec.ResponseSnippet = TruncateSnippet(rec.ResponseSnippet, tc.maxSnip)
	rec.ErrorMessage = TruncateSnippet(rec.ErrorMessage, 1024)
	select {
	case tc.queue <- rec:
		tc.mu.Lock()
		tc.recorded++
		tc.mu.Unlock()
	default:
		tc.mu.Lock()
		tc.dropped++
		tc.mu.Unlock()
		slog.Default().Warn("trace capture queue full, dropping snapshot", "trace_id", rec.TraceID)
	}
}

func (tc *TraceCapture) worker() {
	defer tc.wg.Done()
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	lastPurge := time.Now()

	for {
		select {
		case <-tc.stop:
			tc.drain()
			return
		case rec := <-tc.queue:
			if err := tc.repo.RecordRequestTrace(rec); err != nil {
				slog.Default().Warn("failed to persist request trace", "error", err.Error(), "trace_id", rec.TraceID)
			}
		case <-purge.C:
			if time.Since(lastPurge) < 55*time.Minute {
				continue
			}
			lastPurge = time.Now()
			if n, err := tc.repo.PurgeRequestTraces(time.Now().Add(-tc.ttl)); err == nil && n > 0 {
				slog.Default().Info("purged expired request traces", "count", n)
			}
		}
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

// Stop flushes queued traces and terminates the worker.
func (tc *TraceCapture) Stop() {
	if tc == nil || tc.mode == "off" {
		return
	}
	close(tc.stop)
	tc.wg.Wait()
}
