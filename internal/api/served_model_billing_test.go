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

func servedModelBillingContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-served", GroupName: "default"})
	return c
}

func servedModelBillingRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "alias-model", GroupName: "default",
		PromptPrice: 1, CompletionPrice: 4, CacheReadPrice: 0.1, Currency: "CNY",
	}); err != nil {
		t.Fatal(err)
	}
	return repo
}

// The upstream invoices the model it served. When the channel rewrites the alias
// and that served model has its own configured price, billing must follow it.
func TestBillingUsesServedModelPriceWhenConfigured(t *testing.T) {
	repo := servedModelBillingRepo(t)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "upstream-model", GroupName: "default",
		PromptPrice: 5, CompletionPrice: 10, CacheReadPrice: 0.5, Currency: "CNY",
	}); err != nil {
		t.Fatal(err)
	}

	prevEngine, prevLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	billing.GlobalEngine = billing.NewEngine(repo)
	storage.GlobalAsyncLogger = nil
	SetServedModelResolver(func(channel, modelName string) string {
		if channel == "ch-mapped" && modelName == "alias-model" {
			return "upstream-model"
		}
		return ""
	})
	defer func() {
		billing.GlobalEngine = prevEngine
		storage.GlobalAsyncLogger = prevLogger
		SetServedModelResolver(nil)
	}()

	cost := RecordUsage(servedModelBillingContext(), AuditRecordParams{
		Model:        "alias-model",
		Channel:      "ch-mapped",
		PromptTokens: 1_000_000,
		TotalTokens:  1_000_000,
		StatusCode:   200,
	})
	if cost != 5 {
		t.Fatalf("cost = %v, want 5 (the served model's rate), not 1 (the alias rate)", cost)
	}
}

// An operator-configured alias price must not be silently replaced by the
// fallback rate card when the served model has no price of its own.
func TestBillingKeepsAliasPriceWhenServedModelUnpriced(t *testing.T) {
	repo := servedModelBillingRepo(t)

	prevEngine, prevLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	billing.GlobalEngine = billing.NewEngine(repo)
	storage.GlobalAsyncLogger = nil
	SetServedModelResolver(func(channel, modelName string) string {
		if channel == "ch-mapped" && modelName == "alias-model" {
			return "never-priced-upstream-model"
		}
		return ""
	})
	defer func() {
		billing.GlobalEngine = prevEngine
		storage.GlobalAsyncLogger = prevLogger
		SetServedModelResolver(nil)
	}()

	cost := RecordUsage(servedModelBillingContext(), AuditRecordParams{
		Model:        "alias-model",
		Channel:      "ch-mapped",
		PromptTokens: 1_000_000,
		TotalTokens:  1_000_000,
		StatusCode:   200,
	})
	if cost != 1 {
		t.Fatalf("cost = %v, want the alias rate 1 (served model has no configured price)", cost)
	}
}
