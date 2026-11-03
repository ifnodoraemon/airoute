package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestResolveUpstreamFailureStatusRequestBudget(t *testing.T) {
	status, errType, errCode, _ := resolveUpstreamFailureStatus(
		fmt.Errorf("attempt trail: %w", router.ErrRequestBudgetExceeded))
	if status != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", status, http.StatusGatewayTimeout)
	}
	if errType != "gateway_error" || errCode != "request_budget_exceeded" {
		t.Fatalf("unexpected code: type=%q code=%q", errType, errCode)
	}
}
