package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestBillingEngine_CostCalculationAndCacheSavings(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	engine := billing.NewEngine(repo)

	// Save specific model pricing
	// deepseek-chat: Prompt 2.0 / 1M, Completion 8.0 / 1M, Cache Read 0.2 / 1M (90% discount)
	err = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           "deepseek-chat",
		PromptPrice:     2.0,
		CompletionPrice: 8.0,
		CacheReadPrice:  0.2,
		FixedPrice:      0.0,
		Currency:        "CNY",
	})
	if err != nil {
		t.Fatalf("failed to save price: %v", err)
	}
	_ = engine.ReloadPrices()

	// Scenario 1: Total prompt = 10,000, completion = 1,000, cached = 8,000 (80% cache hit)
	// Uncached prompt = 2,000.
	// Expected Prompt Cost: (2,000 * 2.0) / 1M = 0.004
	// Expected Cache Cost: (8,000 * 0.2) / 1M = 0.0016
	// Expected Completion Cost: (1,000 * 8.0) / 1M = 0.008
	// Total Expected Cost: 0.004 + 0.0016 + 0.008 = 0.0136
	// Expected Saved Cost: (8,000 * (2.0 - 0.2)) / 1M = 0.0144
	cost, savedCost := engine.CalculateCost("deepseek-chat", 10000, 1000, 8000)

	expectedCost := 0.0136
	if diff := cost - expectedCost; diff > 0.00001 || diff < -0.00001 {
		t.Errorf("cost mismatch: expected %.6f, got %.6f", expectedCost, cost)
	}

	expectedSaved := 0.0144
	if diff := savedCost - expectedSaved; diff > 0.00001 || diff < -0.00001 {
		t.Errorf("saved cost mismatch: expected %.6f, got %.6f", expectedSaved, savedCost)
	}

	// Scenario 2: Fixed price for DALL-E-3
	err = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:      "dall-e-3",
		FixedPrice: 0.28,
		Currency:   "CNY",
	})
	if err != nil {
		t.Fatalf("failed to save dall-e-3 price: %v", err)
	}
	_ = engine.ReloadPrices()

	imageCost, _ := engine.CalculateCost("dall-e-3", 0, 0, 0)
	if imageCost != 0.28 {
		t.Errorf("expected fixed cost 0.28, got %.4f", imageCost)
	}
}

func TestModelRoute_MultiUpstreamConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)

	// Create 2 upstream channels
	ch1 := &storage.ChannelRecord{
		Name:     "upstream-provider-a",
		Type:     "openai",
		BaseURL:  "https://api.provider-a.com/v1",
		APIKey:   "sk-test-a",
		Models:   []string{"other-model"},
		Priority: 1,
		Weight:   50,
		Status:   "active",
	}
	ch2 := &storage.ChannelRecord{
		Name:     "upstream-provider-b",
		Type:     "deepseek",
		BaseURL:  "https://api.provider-b.com/v1",
		APIKey:   "sk-test-b",
		Models:   []string{"other-model"},
		Priority: 1,
		Weight:   50,
		Status:   "active",
	}
	_ = repo.CreateChannel(ch1)
	_ = repo.CreateChannel(ch2)
	_ = sync.ReloadFromDB()

	// Use UpdateModelRoute to configure model "deepseek-r1":
	// Assign ch1 (weight 70, priority 1, mapped model "deepseek-ai/DeepSeek-R1")
	// Assign ch2 (weight 30, priority 2 fallback, mapped model "deepseek-r1")
	pPrice := 4.0
	cPrice := 16.0
	cachePrice := 0.4
	updateReq := controlplane.UpdateModelRouteRequest{
		Model:         "deepseek-r1",
		FallbackModel: "deepseek-v3",
		ProviderUpdates: []controlplane.ModelProviderUpdate{
			{
				ChannelID:   ch1.ID,
				Priority:    1,
				Weight:      70,
				MappedModel: "deepseek-ai/DeepSeek-R1",
			},
			{
				ChannelID:   ch2.ID,
				Priority:    2,
				Weight:      30,
				MappedModel: "deepseek-r1",
			},
		},
		PromptPrice:     &pPrice,
		CompletionPrice: &cPrice,
		CacheReadPrice:  &cachePrice,
	}

	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("POST", "/api/v1/admin/models/routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r := gin.New()
	r.POST("/api/v1/admin/models/routes", adminHandler.UpdateModelRoute)
	r.DELETE("/api/v1/admin/models/routes/:model", adminHandler.DeleteModelRoute)
	r.GET("/api/v1/admin/pricing", adminHandler.GetPricingRates)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("UpdateModelRoute failed with status %d: %s", w.Code, w.Body.String())
	}

	// Verify Dispatcher now routes "deepseek-r1" to both channels!
	matched := dispatcher.GetChannelsForModel("deepseek-r1")
	if len(matched) != 2 {
		t.Fatalf("expected 2 channels matched for deepseek-r1, got %d", len(matched))
	}
	// Verify mapped model
	if matched[0].GetUpstreamModel("deepseek-r1") != "deepseek-ai/DeepSeek-R1" {
		t.Errorf("expected mapped model deepseek-ai/DeepSeek-R1, got %s", matched[0].GetUpstreamModel("deepseek-r1"))
	}
	// Verify fallback model
	if fallback := dispatcher.GetModelFallback("deepseek-r1"); fallback != "deepseek-v3" {
		t.Errorf("expected fallback deepseek-v3, got %s", fallback)
	}

	// Verify Pricing was persisted
	priceRec, err := repo.GetModelPrice("deepseek-r1")
	if err != nil {
		t.Fatalf("failed to get model price: %v", err)
	}
	if priceRec.PromptPrice != 4.0 || priceRec.CacheReadPrice != 0.4 {
		t.Errorf("pricing mismatch: prompt=%.2f, cache=%.2f", priceRec.PromptPrice, priceRec.CacheReadPrice)
	}

	// Now verify DeleteModelRoute unbinds the model cleanly
	delReq := httptest.NewRequest("DELETE", "/api/v1/admin/models/routes/deepseek-r1", nil)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("DeleteModelRoute failed: status %d, body: %s", delW.Code, delW.Body.String())
	}

	matchedAfter := dispatcher.GetChannelsForModel("deepseek-r1")
	if len(matchedAfter) != 0 {
		t.Errorf("expected 0 channels matched after deletion, got %d", len(matchedAfter))
	}
}
