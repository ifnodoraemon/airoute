package controlplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Pins the HTTP contract of the pricing endpoints. The enriched row shape in
// particular moved out of the handler and into PricingService, so its JSON tags
// are asserted here: a renamed tag would break the console silently.

func newPricingTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func invokePricing(t *testing.T, handler gin.HandlerFunc, method, body, url string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, url, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if params != nil {
		c.Params = params
	}
	handler(c)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON (%d): %s", w.Code, w.Body.String())
	}
	return out
}

func TestGetPricingRatesHTTPContract(t *testing.T) {
	handler, repo := newPricingTestHandler(t)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "gpt-x", GroupName: "default", PromptPrice: 1.25, CompletionPrice: 2.5, Currency: "CNY",
	}); err != nil {
		t.Fatalf("seed price: %v", err)
	}

	w := invokePricing(t, handler.GetPricingRates, http.MethodGet, "", "/api/v1/admin/pricing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	env := decodeBody(t, w)
	if code, _ := env["code"].(float64); code != 0 {
		t.Errorf("envelope code = %v, want 0", env["code"])
	}
	// The clock context the console renders off-peak state against.
	for _, key := range []string{"server_time", "server_weekday", "is_weekend"} {
		if _, ok := env[key]; !ok {
			t.Errorf("response is missing %q", key)
		}
	}

	rows, ok := env["data"].([]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("data = %v, want a one-element array", env["data"])
	}
	row, ok := rows[0].(map[string]interface{})
	if !ok {
		t.Fatalf("row is not an object: %v", rows[0])
	}
	// The embedded record's fields must still be inlined...
	for _, key := range []string{"model", "group_name", "prompt_price", "completion_price", "currency"} {
		if _, ok := row[key]; !ok {
			t.Errorf("row is missing the embedded field %q", key)
		}
	}
	// ...alongside the enrichment fields, under the names the console reads.
	for _, key := range []string{
		"is_currently_off_peak", "current_discount",
		"effective_prompt_price", "effective_completion_price",
	} {
		if _, ok := row[key]; !ok {
			t.Errorf("row is missing the enrichment field %q", key)
		}
	}
	if row["model"] != "gpt-x" {
		t.Errorf("model = %v, want gpt-x", row["model"])
	}
}

func TestSavePricingRateHTTPContract(t *testing.T) {
	t.Run("saves and persists", func(t *testing.T) {
		handler, repo := newPricingTestHandler(t)

		w := invokePricing(t, handler.SavePricingRate, http.MethodPost,
			`{"model":"gpt-x","group_name":"default","prompt_price":3,"completion_price":4}`,
			"/api/v1/admin/pricing", nil)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}
		if msg, _ := decodeBody(t, w)["message"].(string); !strings.Contains(msg, "已保存") {
			t.Errorf("message = %q, want a saved confirmation", msg)
		}

		stored, err := repo.GetModelPriceExact("gpt-x", "default")
		if err != nil {
			t.Fatalf("price was not persisted: %v", err)
		}
		if stored.PromptPrice != 3 || stored.CompletionPrice != 4 {
			t.Errorf("stored = %+v, want prompt 3 / completion 4", stored)
		}
	})

	t.Run("rejects an empty model with 400", func(t *testing.T) {
		handler, _ := newPricingTestHandler(t)

		w := invokePricing(t, handler.SavePricingRate, http.MethodPost,
			`{"model":"   "}`, "/api/v1/admin/pricing", nil)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if msg, _ := decodeBody(t, w)["error"].(string); msg != "模型标识为必填项" {
			t.Errorf("error = %q, want the documented message", msg)
		}
	})

	t.Run("rejects overlapping off-peak slots with 400", func(t *testing.T) {
		handler, _ := newPricingTestHandler(t)

		w := invokePricing(t, handler.SavePricingRate, http.MethodPost,
			`{"model":"gpt-x","off_peak_enabled":true,"off_peak_slots":"00:00-10:00,05:00-12:00"}`,
			"/api/v1/admin/pricing", nil)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
		}
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		handler, _ := newPricingTestHandler(t)

		w := invokePricing(t, handler.SavePricingRate, http.MethodPost, `{nope`, "/api/v1/admin/pricing", nil)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestDeletePricingRateHTTPContract(t *testing.T) {
	handler, repo := newPricingTestHandler(t)
	for _, group := range []string{"default", "vip"} {
		if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
			Model: "gpt-x", GroupName: group, PromptPrice: 1, Currency: "CNY",
		}); err != nil {
			t.Fatalf("seed %s: %v", group, err)
		}
	}

	t.Run("deletes one group only", func(t *testing.T) {
		w := invokePricing(t, handler.DeletePricingRate, http.MethodDelete, "",
			"/api/v1/admin/pricing/gpt-x?group=vip", gin.Params{{Key: "model", Value: "gpt-x"}})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}
		if _, err := repo.GetModelPriceExact("gpt-x", "vip"); err == nil {
			t.Error("the vip row survived a group-scoped delete")
		}
		if _, err := repo.GetModelPriceExact("gpt-x", "default"); err != nil {
			t.Errorf("the default row was deleted too: %v", err)
		}
	})

	t.Run("rejects an empty model with 400", func(t *testing.T) {
		w := invokePricing(t, handler.DeletePricingRate, http.MethodDelete, "",
			"/api/v1/admin/pricing/%20", gin.Params{{Key: "model", Value: "  "}})

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestBatchDeletePricingRatesHTTPContract(t *testing.T) {
	t.Run("rejects an empty selection with 400", func(t *testing.T) {
		handler, _ := newPricingTestHandler(t)

		w := invokePricing(t, handler.BatchDeletePricingRates, http.MethodPost,
			`{"models":[],"items":[]}`, "/api/v1/admin/pricing/batch-delete", nil)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if msg, _ := decodeBody(t, w)["error"].(string); msg != "请提供待删除定价模型列表" {
			t.Errorf("error = %q, want the documented message", msg)
		}
	})

	t.Run("deletes by model list", func(t *testing.T) {
		handler, repo := newPricingTestHandler(t)
		if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
			Model: "gpt-x", GroupName: "default", PromptPrice: 1, Currency: "CNY",
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		w := invokePricing(t, handler.BatchDeletePricingRates, http.MethodPost,
			`{"models":["gpt-x"]}`, "/api/v1/admin/pricing/batch-delete", nil)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}
		if _, err := repo.GetModelPriceExact("gpt-x", "default"); err == nil {
			t.Error("the rate survived the batch delete")
		}
	})

	t.Run("storage failure reports 500 with the raw cause", func(t *testing.T) {
		handler, _ := newPricingTestHandler(t)
		handler.pricing = NewPricingService(
			&fakePricingStore{batchModelsErr: errors.New("boom")}, nil, nil, nil)

		w := invokePricing(t, handler.BatchDeletePricingRates, http.MethodPost,
			`{"models":["gpt-x"]}`, "/api/v1/admin/pricing/batch-delete", nil)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
		if msg, _ := decodeBody(t, w)["error"].(string); msg != "boom" {
			t.Errorf("error = %q, want the raw cause", msg)
		}
	})
}

func TestSavePricingRateStorageFailureMessage(t *testing.T) {
	handler, _ := newPricingTestHandler(t)
	handler.pricing = NewPricingService(
		&fakePricingStore{saveErr: errors.New("boom")}, nil, nil, nil)

	w := invokePricing(t, handler.SavePricingRate, http.MethodPost,
		`{"model":"gpt-x"}`, "/api/v1/admin/pricing", nil)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if msg, _ := decodeBody(t, w)["error"].(string); msg != "保存模型定价失败: boom" {
		t.Errorf("error = %q, want the documented prefix", msg)
	}
}

func TestPricingGroupMultipliersEndpoints(t *testing.T) {
	prevEngine := billing.GlobalEngine
	defer func() { billing.GlobalEngine = prevEngine }()

	handler, repo := newPricingTestHandler(t)
	billing.InitGlobalEngine(repo)

	// 1. Get pricing groups
	w := invokePricing(t, handler.GetPricingGroups, http.MethodGet, "", "/api/v1/admin/pricing/groups", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %v", body["data"])
	}
	if data["default"] != 1.0 {
		t.Errorf("expected default=1.0, got %v", data["default"])
	}

	// 2. Save custom group multipliers
	payload := `{"default": 1.0, "vip": 0.75, "enterprise": 1.6, "custom_tier": 0.5}`
	wSave := invokePricing(t, handler.SavePricingGroups, http.MethodPost, payload, "/api/v1/admin/pricing/groups", nil)
	if wSave.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200", wSave.Code)
	}
	saveBody := decodeBody(t, wSave)
	saveData, _ := saveBody["data"].(map[string]interface{})
	if saveData["vip"] != 0.75 || saveData["custom_tier"] != 0.5 {
		t.Errorf("saved multipliers mismatch: %v", saveData)
	}

	// 3. Overview should include group_multipliers
	wOverview := invokePricing(t, handler.GetPricingRates, http.MethodGet, "", "/api/v1/admin/pricing", nil)
	overviewBody := decodeBody(t, wOverview)
	gm, ok := overviewBody["group_multipliers"].(map[string]interface{})
	if !ok {
		t.Fatalf("overview missing group_multipliers: %v", overviewBody)
	}
	if gm["vip"] != 0.75 {
		t.Errorf("overview group_multipliers vip = %v, want 0.75", gm["vip"])
	}
}
