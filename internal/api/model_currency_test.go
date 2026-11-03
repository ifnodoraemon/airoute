package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// withPricedEngine installs a billing engine backed by repo and restores the
// previous globals afterwards.
func withPricedEngine(t *testing.T, repo *storage.Repository) *billing.BillingEngine {
	t.Helper()
	previousEngine := billing.GlobalEngine
	previousEnumerator := servedModelsForModel
	engine := billing.NewEngine(repo)
	billing.GlobalEngine = engine
	t.Cleanup(func() {
		billing.GlobalEngine = previousEngine
		servedModelsForModel = previousEnumerator
	})
	return engine
}

func newCurrencyTestRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return storage.NewRepository(db)
}

// forceForeignPrice simulates a rate row persisted before the CNY-only write
// validation existed: model prices can only be written as CNY today, so a legacy
// foreign row has to be produced at the SQL level.
func forceForeignPrice(t *testing.T, repo *storage.Repository, modelName string) {
	t.Helper()
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: modelName, GroupName: "default", PromptPrice: 1, CompletionPrice: 1, Currency: "CNY",
	}); err != nil {
		t.Fatalf("save %s price: %v", modelName, err)
	}
	if _, err := repo.GetDB().Exec(`UPDATE model_prices SET currency='USD' WHERE model=?`, modelName); err != nil {
		t.Fatalf("force foreign currency for %s: %v", modelName, err)
	}
}

// C-14: the gate must cover the model settlement will actually price. A channel
// mapping to a served model with its own non-CNY rate card used to pass this
// check and only surface as a zeroed charge after the upstream work was paid for.
func TestValidateModelCurrencyCoversServedModel(t *testing.T) {
	repo := newCurrencyTestRepo(t)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "alias-model", GroupName: "default", PromptPrice: 1, CompletionPrice: 1, Currency: "CNY",
	}); err != nil {
		t.Fatalf("save alias price: %v", err)
	}
	forceForeignPrice(t, repo, "upstream-model")
	withPricedEngine(t, repo)

	// Without the channel mapping only the requested alias is priced, so the
	// request is billable and must be admitted.
	if got := currencyModelsForBilling("alias-model", "default"); len(got) != 1 || got[0] != "alias-model" {
		t.Fatalf("without an enumerator candidates = %v, want [alias-model]", got)
	}

	// With the mapping installed, billing would price the served model, so the
	// gate must inspect it too.
	servedModelsForModel = func(string) []string { return []string{"upstream-model"} }
	candidates := currencyModelsForBilling("alias-model", "default")
	if len(candidates) != 2 || candidates[1] != "upstream-model" {
		t.Fatalf("candidates = %v, want alias + served model", candidates)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	if validateModelCurrency(c, "alias-model") {
		t.Fatal("a non-CNY served model price must be rejected before dispatch")
	}
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

// A served model without its own rate card does not change what billing prices, so
// it must not turn a valid request into a 422.
func TestValidateModelCurrencyIgnoresUnpricedServedModel(t *testing.T) {
	repo := newCurrencyTestRepo(t)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "alias-model", GroupName: "default", PromptPrice: 1, CompletionPrice: 1, Currency: "CNY",
	}); err != nil {
		t.Fatalf("save alias price: %v", err)
	}
	withPricedEngine(t, repo)
	servedModelsForModel = func(string) []string { return []string{"unpriced-upstream", "alias-model"} }

	if got := currencyModelsForBilling("alias-model", "default"); len(got) != 1 {
		t.Fatalf("candidates = %v, want only the requested model", got)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if !validateModelCurrency(c, "alias-model") {
		t.Fatalf("priced alias must pass, got status %d", w.Code)
	}
}
