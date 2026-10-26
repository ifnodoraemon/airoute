package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// A canceled request context means the downstream went away. The dispatcher
// returns ctx.Err() when it notices, and that used to fall through to the
// generic 502 "upstream failure" label: 259 audit rows blamed the provider for
// clients hanging up, and every abort counted against the gateway error rate.
func TestClientCancellationIsNotAnUpstreamFailure(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantCode  int
		wantType  string
		wantErrID string
	}{
		{"canceled", context.Canceled, statusClientClosed, "client_closed_request", "client_canceled"},
		{"wrapped-canceled", fmt.Errorf("dispatch: %w", context.Canceled), statusClientClosed, "client_closed_request", "client_canceled"},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout, "gateway_error", "upstream_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, errType, errCode, _ := resolveUpstreamFailureStatus(tc.err)
			if code != tc.wantCode || errType != tc.wantType || errCode != tc.wantErrID {
				t.Fatalf("got (%d, %q, %q), want (%d, %q, %q)", code, errType, errCode, tc.wantCode, tc.wantType, tc.wantErrID)
			}
		})
	}

	// A genuine upstream failure must keep its 502 label.
	code, _, errCode, _ := resolveUpstreamFailureStatus(errors.New("connection reset by peer"))
	if code != http.StatusBadGateway || errCode != "upstream_failure" {
		t.Fatalf("upstream failure got (%d, %q), want (502, upstream_failure)", code, errCode)
	}
}

// Aborted requests must not be counted as gateway failures.
func TestClientAbortIsCountedAsCanceledNotFailed(t *testing.T) {
	setUpAbortBilling(t)
	beforeCanceled := telemetry.GlobalMetrics.ClientCanceledRequests.Load()
	beforeFailed := telemetry.GlobalMetrics.FailedRequests.Load()

	RecordUsage(abortBillingContext(), AuditRecordParams{
		Model: "abort-model", PromptTokens: 1_000_000, TotalTokens: 1_000_000, StatusCode: statusClientClosed,
	})

	if got := telemetry.GlobalMetrics.ClientCanceledRequests.Load(); got != beforeCanceled+1 {
		t.Fatalf("client canceled counter = %d, want %d", got, beforeCanceled+1)
	}
	if got := telemetry.GlobalMetrics.FailedRequests.Load(); got != beforeFailed {
		t.Fatalf("failed counter moved to %d (was %d): a client abort must not count as a failure", got, beforeFailed)
	}
}
