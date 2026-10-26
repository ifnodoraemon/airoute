package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// setUpAbortBilling prices one model at ¥2 / 1M prompt tokens and installs it as
// the global engine, so a cost of 2.0 means "one million prompt tokens were
// billed at the configured rate".
func setUpAbortBilling(t *testing.T) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "abort-model", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY",
	}); err != nil {
		t.Fatal(err)
	}
	oldEngine, oldLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	billing.GlobalEngine = billing.NewEngine(repo)
	storage.GlobalAsyncLogger = nil
	t.Cleanup(func() { billing.GlobalEngine = oldEngine; storage.GlobalAsyncLogger = oldLogger })
}

func abortBillingContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-abort-synthetic", GroupName: "default"})
	return c
}

// A client that disconnects after the upstream produced tokens must still be
// billed: the gateway paid for those tokens. Previously every 499 short-circuited
// to cost 0, making "read the answer, then hang up" free usage.
func TestClientAbortAfterUsageIsBilled(t *testing.T) {
	setUpAbortBilling(t)
	got := RecordUsage(abortBillingContext(), AuditRecordParams{
		Model: "abort-model", PromptTokens: 1_000_000, TotalTokens: 1_000_000, StatusCode: statusClientClosed,
	})
	if got != 2 {
		t.Fatalf("client abort with consumed tokens cost = %v, want 2 (billed at the configured rate)", got)
	}
}

// An abort before any token was produced has nothing to bill.
func TestClientAbortWithoutUsageIsFree(t *testing.T) {
	setUpAbortBilling(t)
	got := RecordUsage(abortBillingContext(), AuditRecordParams{
		Model: "abort-model", StatusCode: statusClientClosed,
	})
	if got != 0 {
		t.Fatalf("client abort with no consumed tokens cost = %v, want 0", got)
	}
}

// Upstream failures keep the old policy: the customer is not charged for a
// request the gateway could not serve.
func TestUpstreamFailureIsNotBilled(t *testing.T) {
	setUpAbortBilling(t)
	for _, status := range []int{502, 503, 429, 400} {
		got := RecordUsage(abortBillingContext(), AuditRecordParams{
			Model: "abort-model", PromptTokens: 1_000_000, TotalTokens: 1_000_000, StatusCode: status,
		})
		if got != 0 {
			t.Fatalf("status %d cost = %v, want 0", status, got)
		}
	}
}

// Successful requests keep billing exactly as before.
func TestSuccessfulRequestStillBilled(t *testing.T) {
	setUpAbortBilling(t)
	got := RecordUsage(abortBillingContext(), AuditRecordParams{
		Model: "abort-model", PromptTokens: 1_000_000, TotalTokens: 1_000_000, StatusCode: 200,
	})
	if got != 2 {
		t.Fatalf("successful request cost = %v, want 2", got)
	}
}
