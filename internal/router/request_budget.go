package router

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// RequestBudgetEnv bounds how long a non-streaming dispatch may spend across
// channel attempts and cross-model fallback. Streaming keeps the first-token
// timeout instead: a total deadline would cut long generations that are working
// as intended.
const RequestBudgetEnv = "GATEWAY_REQUEST_BUDGET_SEC"

// DefaultRequestBudget is generous enough for the default timeout chain
// (channel timeout × candidate channels) while still bounding a request that
// would otherwise wait for every candidate to time out.
const DefaultRequestBudget = 300 * time.Second

// ErrRequestBudgetExceeded marks a request stopped by the gateway's own total
// budget rather than by an upstream failure. The HTTP layer maps it to 504 with
// a distinct code so operators can tell "we gave up" from "the provider failed".
var ErrRequestBudgetExceeded = errors.New("request budget exhausted")

// requestBudgetFromEnv reads GATEWAY_REQUEST_BUDGET_SEC. Unset uses the
// default; zero or a negative value disables the budget.
func requestBudgetFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(RequestBudgetEnv))
	if raw == "" {
		return DefaultRequestBudget
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// requestBudgetContextKey marks a context whose deadline was imposed by this
// gateway's total budget. A caller-supplied deadline must not be reported as a
// budget exhaustion (that would blame the gateway for the caller's own timeout).
type requestBudgetContextKey struct{}

func hasRequestBudget(ctx context.Context) bool {
	marked, _ := ctx.Value(requestBudgetContextKey{}).(bool)
	return marked
}

func requestBudgetExceeded(ctx context.Context, err error) bool {
	return hasRequestBudget(ctx) && errors.Is(err, context.DeadlineExceeded)
}

// withRequestBudget derives a deadline from ctx unless the caller already set
// one (a shorter caller deadline is respected). Returns ctx unchanged and a
// no-op cancel when the budget is disabled.
func withRequestBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	budget := requestBudgetFromEnv()
	if budget <= 0 {
		return ctx, func() {}
	}
	timed, cancel := context.WithTimeout(ctx, budget)
	return context.WithValue(timed, requestBudgetContextKey{}, true), cancel
}

// newRequestBudgetError builds an error that unwraps to ErrRequestBudgetExceeded
// and carries the attempted-channel trail for the audit log.
func newRequestBudgetError(modelName string, attempted []string, lastErr error) error {
	detail := fmt.Sprintf("request budget exhausted for model %s after %d channel attempt(s)", modelName, len(attempted))
	if len(attempted) > 0 {
		detail += " (" + strings.Join(attempted, "→") + ")"
	}
	if lastErr != nil {
		detail += ": " + lastErr.Error()
	}
	return fmt.Errorf("%s: %w", detail, ErrRequestBudgetExceeded)
}
