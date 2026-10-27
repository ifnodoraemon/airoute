package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// canceledClientContext returns a gin context whose downstream request context
// is already canceled, i.e. the client has hung up before the handler finished.
func canceledClientContext(t *testing.T) *gin.Context {
	t.Helper()
	c := abortBillingContext()
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	return c
}

// initTraceCaptureForTest installs an in-memory trace archiver and returns a
// flush helper. Tests call the helper before querying request_traces; the
// cleanup stops the worker exactly once so the repository is still open.
func initTraceCaptureForTest(t *testing.T, repo *storage.Repository) (*storage.TraceCapture, func()) {
	t.Helper()
	t.Setenv("GATEWAY_TRACE_CAPTURE", "errors")
	previous := storage.GlobalTraceCapture
	tc := storage.InitTraceCapture(repo)
	var once sync.Once
	stop := func() { once.Do(tc.Stop) }
	t.Cleanup(func() {
		stop()
		storage.GlobalTraceCapture = previous
	})
	return tc, stop
}

// The synthesized 499 explanation must be accurate for every kind of abort:
// billed tokens, estimated tokens, fixed-price units, and an abort before any
// usage arrived. It must always read as a client-side disconnect, never as an
// upstream failure, and it must survive the audit scrubber untouched.
func TestClientClosedAuditReason(t *testing.T) {
	cases := []struct {
		name     string
		params   AuditRecordParams
		contains []string
		absent   []string
	}{
		{
			name: "billed completion tokens",
			params: AuditRecordParams{
				StatusCode: statusClientClosed, PromptTokens: 1200, CompletionTokens: 24000,
			},
			contains: []string{"24000 completion tokens", "1200 prompt tokens", "billed"},
		},
		{
			name: "estimated counts are called out",
			params: AuditRecordParams{
				StatusCode: statusClientClosed, PromptTokens: 1200, CompletionTokens: 24000,
				UsageSource: usageSourceEstimated,
			},
			contains: []string{"estimated", "did not report usage"},
		},
		{
			name: "fixed-price units",
			params: AuditRecordParams{
				StatusCode: statusClientClosed, BillableUnits: 3,
			},
			contains: []string{"3 billable units", "billed"},
		},
		{
			name: "no provider usage at all",
			params: AuditRecordParams{
				StatusCode: statusClientClosed,
			},
			contains: []string{"before any provider usage was received"},
			absent:   []string{"billed", "estimated"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := clientClosedAuditReason(tc.params)
			if !strings.HasPrefix(reason, "client closed request") {
				t.Fatalf("reason = %q, want the stable %q prefix", reason, "client closed request")
			}
			if !strings.Contains(reason, "synthesized by the gateway") {
				t.Errorf("reason = %q, want it to say 499 is gateway-synthesized", reason)
			}
			if !strings.Contains(reason, "not a status code returned by the upstream provider") {
				t.Errorf("reason = %q, want it to disclaim an upstream status", reason)
			}
			lower := strings.ToLower(reason)
			for _, word := range []string{"error", "failed", "failure"} {
				if strings.Contains(lower, word) {
					t.Errorf("reason = %q, must never imply an upstream failure (found %q)", reason, word)
				}
			}
			for _, want := range tc.contains {
				if !strings.Contains(reason, want) {
					t.Errorf("reason = %q, want it to contain %q", reason, want)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(reason, unwanted) {
					t.Errorf("reason = %q, want it NOT to contain %q", reason, unwanted)
				}
			}
			if scrubbed := auditErrorMessage(reason); scrubbed != reason {
				t.Errorf("audit scrubber rewrote the synthesized reason: got %q, want %q", scrubbed, reason)
			}
		})
	}

	// The helper only explains 499; other statuses keep their handler-provided
	// message untouched.
	if got := clientClosedAuditReason(AuditRecordParams{StatusCode: http.StatusBadGateway}); got != "" {
		t.Fatalf("reason for a 502 = %q, want empty (only 499 is synthesized)", got)
	}
}

// The incident: the client timed out at ~120s, the provider had already
// produced (and billed) 24000 completion tokens, and the audit row read
// "status=499" with an empty error_message — indistinguishable from an upstream
// fault. The forced flip must persist a self-explaining message.
func TestCanceledContextAuditRowExplainsClientAbort(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "audit-499-model", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY",
	})
	c := canceledClientContext(t)

	// The handler reported success; the recorder flips it to 499.
	RecordUsage(c, AuditRecordParams{
		Model: "audit-499-model", Channel: "ch-audit-499",
		PromptTokens: 1200, CompletionTokens: 24000, TotalTokens: 25200,
		StatusCode: http.StatusOK, Duration: 120 * time.Second,
	})

	row := edgeUsageRows(t, repo, 1)[0]
	if row.StatusCode != statusClientClosed {
		t.Fatalf("audit status = %d, want %d", row.StatusCode, statusClientClosed)
	}
	if !strings.HasPrefix(row.ErrorMessage, "client closed request") {
		t.Fatalf("error_message = %q, want the %q prefix", row.ErrorMessage, "client closed request")
	}
	for _, want := range []string{"24000 completion tokens", "1200 prompt tokens", "billed"} {
		if !strings.Contains(row.ErrorMessage, want) {
			t.Errorf("error_message = %q, want it to contain %q", row.ErrorMessage, want)
		}
	}
	if strings.Contains(strings.ToLower(row.ErrorMessage), "error") {
		t.Errorf("error_message = %q claims an upstream error; a client abort must not blame the provider", row.ErrorMessage)
	}

	// The access-log middleware reads these two keys after the handler returns:
	// the wire status stayed 200, so only the audit status reveals the abort.
	if got := c.GetInt(middleware.ContextKeyAuditStatus); got != statusClientClosed {
		t.Errorf("context audit status = %d, want %d", got, statusClientClosed)
	}
	if got := c.GetString(middleware.ContextKeyErrorMessage); !strings.Contains(got, "client closed request") {
		t.Errorf("context error message = %q, want the synthesized reason", got)
	}
}

// A 499 passed directly by a handler (drained stream, dispatch cancellation with
// an empty message) gets the same explanation as the forced flip — the incident
// proves both paths reach operators as an empty row otherwise.
func TestDirectClientClosedAuditRowExplainsItself(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "direct-499-model", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, Currency: "CNY",
	})
	c := abortBillingContext() // context is NOT canceled: the handler chose 499

	RecordUsage(c, AuditRecordParams{
		Model: "direct-499-model", Channel: "ch-direct-499",
		PromptTokens: 100, CompletionTokens: 40,
		StatusCode: statusClientClosed, UsageSource: usageSourceEstimated,
	})

	row := edgeUsageRows(t, repo, 1)[0]
	if row.StatusCode != statusClientClosed {
		t.Fatalf("audit status = %d, want %d", row.StatusCode, statusClientClosed)
	}
	if !strings.Contains(row.ErrorMessage, "client closed request") {
		t.Fatalf("error_message = %q, want a synthesized reason for a direct 499", row.ErrorMessage)
	}
	if !strings.Contains(row.ErrorMessage, "estimated") {
		t.Errorf("error_message = %q, want the estimated usage_source called out", row.ErrorMessage)
	}
}

// A client that left before the provider produced anything must say exactly
// that, instead of borrowing the "billed" wording.
func TestClientClosedWithoutUsageSaysNothingWasReceived(t *testing.T) {
	repo := edgeBillingRepo(t)
	c := canceledClientContext(t)

	RecordUsage(c, AuditRecordParams{Model: "empty-499-model", StatusCode: http.StatusOK})

	row := edgeUsageRows(t, repo, 1)[0]
	if row.StatusCode != statusClientClosed {
		t.Fatalf("audit status = %d, want %d", row.StatusCode, statusClientClosed)
	}
	if !strings.Contains(row.ErrorMessage, "before any provider usage was received") {
		t.Fatalf("error_message = %q, want it to state that no provider usage was received", row.ErrorMessage)
	}
}

// Non-HTTP entrypoints (the MCP transport) record usage with a nil gin context.
// The 499 branch writes an audit status and a trace onto that context, so it must
// stay nil-safe: no panic, no trace row, but the audit reason is still persisted.
func TestClientClosedAuditNilContextIsSafe(t *testing.T) {
	repo := edgeBillingRepo(t)
	_, stop := initTraceCaptureForTest(t, repo)

	RecordUsage(nil, AuditRecordParams{
		Model: "nil-ctx-model", PromptTokens: 5, StatusCode: statusClientClosed,
	})
	stop()

	row := edgeUsageRows(t, repo, 1)[0]
	if !strings.HasPrefix(row.ErrorMessage, "client closed request") {
		t.Fatalf("error_message = %q, want a synthesized reason even without a gin context", row.ErrorMessage)
	}
	traces, err := repo.ListRequestTraces(storage.TraceFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 0 {
		t.Fatalf("trace rows = %d, want 0 without a gin context", len(traces))
	}
}

// The console joins a 499 on its trace snapshot; before this, the incident's
// Trace ID was empty. The audit path must archive exactly one trace per request,
// even when a dispatch failure already archived one.
func TestClientClosedAuditRecordsTraceOnce(t *testing.T) {
	t.Run("recorded from the audit path", func(t *testing.T) {
		repo := edgeBillingRepo(t)
		_, stop := initTraceCaptureForTest(t, repo)
		c := canceledClientContext(t)

		RecordUsage(c, AuditRecordParams{
			Model: "trace-499-model", Channel: "ch-trace-499",
			PromptTokens: 1200, CompletionTokens: 24000, TotalTokens: 25200,
			StatusCode: http.StatusOK, Duration: time.Second,
		})
		stop()

		traces, err := repo.ListRequestTraces(storage.TraceFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(traces) != 1 {
			t.Fatalf("trace rows = %d, want exactly 1", len(traces))
		}
		if traces[0].StatusCode != statusClientClosed {
			t.Errorf("trace status = %d, want %d", traces[0].StatusCode, statusClientClosed)
		}
		if !strings.Contains(traces[0].ErrorMessage, "client closed request") {
			t.Errorf("trace error_message = %q, want the synthesized 499 reason", traces[0].ErrorMessage)
		}
		if traces[0].Model != "trace-499-model" || traces[0].Channel != "ch-trace-499" {
			t.Errorf("trace model/channel = %q/%q, want the audit values", traces[0].Model, traces[0].Channel)
		}
	})

	t.Run("existing dispatch trace is not duplicated", func(t *testing.T) {
		repo := edgeBillingRepo(t)
		_, stop := initTraceCaptureForTest(t, repo)
		c := canceledClientContext(t)

		// The dispatch path archives a richer message before the usage recorder runs.
		RecordTrace(c, "trace-499-model", "ch-trace-499", statusClientClosed, "dispatch: context canceled", "", time.Second)
		RecordUsage(c, AuditRecordParams{
			Model: "trace-499-model", Channel: "ch-trace-499",
			PromptTokens: 10, CompletionTokens: 5, StatusCode: http.StatusOK,
		})
		stop()

		traces, err := repo.ListRequestTraces(storage.TraceFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(traces) != 1 {
			t.Fatalf("trace rows = %d, want exactly 1 (the existing dispatch trace must win)", len(traces))
		}
		if traces[0].ErrorMessage != "dispatch: context canceled" {
			t.Errorf("trace error_message = %q, want the dispatch message kept", traces[0].ErrorMessage)
		}
	})

	t.Run("repeated recordUsage keeps one trace", func(t *testing.T) {
		repo := edgeBillingRepo(t)
		_, stop := initTraceCaptureForTest(t, repo)
		c := canceledClientContext(t)
		params := AuditRecordParams{
			Model: "trace-499-model", PromptTokens: 10, CompletionTokens: 5, StatusCode: http.StatusOK,
		}

		RecordUsage(c, params)
		RecordUsage(c, params)
		stop()

		traces, err := repo.ListRequestTraces(storage.TraceFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(traces) != 1 {
			t.Fatalf("trace rows = %d, want exactly 1", len(traces))
		}
	})
}

// Regression for the reported incident: the stdout access line said "status=200"
// with no error while the audit row for the same trace_id was a bare 499 with an
// empty error_message, so nobody could see that the client had aborted after the
// provider produced and billed 24000 completion tokens.
func TestIncidentAccessLogLineShowsClientAbort(t *testing.T) {
	edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "incident-model", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, Currency: "CNY",
	})

	var buf bytes.Buffer
	previous := telemetry.Logger
	telemetry.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	t.Cleanup(func() { telemetry.Logger = previous })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AccessLogMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		// The downstream hangs up while the provider call is in flight; the
		// handler then finishes normally and reports a 200.
		ctx, cancel := context.WithCancel(c.Request.Context())
		cancel()
		c.Request = c.Request.WithContext(ctx)
		RecordUsage(c, AuditRecordParams{
			Model: "incident-model", Channel: "ch-incident",
			PromptTokens: 1200, CompletionTokens: 24000, TotalTokens: 25200,
			StatusCode: http.StatusOK, Duration: 120 * time.Second,
		})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.7:4567"
	r.ServeHTTP(httptest.NewRecorder(), req)

	entry := accessLogEntry(t, buf.String())
	if got := entry["status"]; got != float64(http.StatusOK) {
		t.Fatalf("wire status = %v, want 200 (the response head was already sent)", got)
	}
	if got := entry["audit_status"]; got != float64(statusClientClosed) {
		t.Fatalf("audit_status = %v, want 499", got)
	}
	if got, _ := entry["error"].(string); !strings.Contains(got, "client closed request") {
		t.Fatalf("error = %q, want the synthesized client-abort reason", got)
	}
}

// accessLogEntry returns the parsed http_access line of a JSON log buffer.
func accessLogEntry(t *testing.T, raw string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("access log line is not JSON: %q (%v)", line, err)
		}
		if entry["msg"] == "http_access" {
			return entry
		}
	}
	t.Fatalf("no http_access line was emitted; log was %q", raw)
	return nil
}
