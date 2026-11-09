package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestQuotaBatchSumsAndBoundsEveryInlineRequest(t *testing.T) {
	var body any
	decoder := json.NewDecoder(strings.NewReader(`{"batch":{"inputConfig":{"requests":{"requests":[{"request":{"contents":[{"parts":[{"text":"hi"}]}]}},{"request":{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":100}}}]}}}}`))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		t.Fatal(err)
	}
	prompt, output, count, err := boundBatchRequests("quota-model", "batchGenerateContent", body)
	if err != nil || count != 2 || prompt <= 0 || output != 4196 {
		t.Fatalf("batch bound: prompt=%d output=%d count=%d err=%v", prompt, output, count, err)
	}
	bounded, _ := json.Marshal(body)
	if !strings.Contains(string(bounded), `"maxOutputTokens":4096`) {
		t.Fatalf("wire has no default cap: %s", bounded)
	}
}

func TestLiveQuotaRejectsInputBeforeItCanReachProvider(t *testing.T) {
	_, repo, key := quotaHTTPFixture(t, 0.1, 0, func(http.ResponseWriter, *http.Request) { t.Error("unexpected provider call") })
	quota, err := middleware.AcquireRequestQuota(context.Background(), repo, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer quota.Close()
	meter := &liveQuotaMeter{quota: quota, name: "quota-model", group: "default", cap: 4096}
	if err := meter.reserve(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"clientContent": map[string]any{"turns": []any{map[string]any{"parts": []any{map[string]string{"text": strings.Repeat("x", 200000)}}}}}})
	if err := meter.admitInput(context.Background(), data); !errors.Is(err, storage.ErrWalletReserved) {
		t.Fatalf("unfunded Live input admitted: %v", err)
	}
}

func TestQuotaAsyncJobRetainsAndReleasesOriginalReservation(t *testing.T) {
	for _, tokens := range []int{0, 100} {
		t.Run(fmt.Sprint(tokens), func(t *testing.T) {
			engine, repo, key := quotaHTTPFixture(t, 1, 0, func(http.ResponseWriter, *http.Request) {})
			resource := &googleResource{namespace: "gemini-batch", collection: "batches", model: "quota-model",
				channel: &model.ChannelConfig{Name: "saved-job", BaseURL: "http://job.invalid", Type: model.ProviderGemini}}
			engine.POST("/test/job", func(c *gin.Context) {
				if err := saveGoogleResource(c, resource, "batches/funded-job", []byte(`{"name":"batches/funded-job","done":false}`)); err != nil {
					t.Error(err)
				}
				c.Status(200)
			})
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test/job", nil))
			if w.Code != 200 {
				t.Fatalf("job creation: %d %s", w.Code, w.Body.String())
			}
			if _, err := repo.ReserveBilling(context.Background(), key.Key, storage.ToUnits(1), 64); !errors.Is(err, storage.ErrWalletReserved) {
				t.Fatalf("creating response released the job's hold: %v", err)
			}
			params := googleUsageParams(nil, resource)
			params.Model, params.Channel, params.StatusCode = "quota-model", "saved-job", 200
			params.BillingID, params.CompletionTokens, params.TotalTokens = "finished-funded-job", tokens, tokens
			RecordUsage(nil, params)
			want := 1 - float64(tokens)*10/1_000_000
			hold, err := repo.ReserveBilling(context.Background(), key.Key, storage.ToUnits(want), 64)
			if err != nil {
				t.Fatalf("final settlement leaked the job's hold: %v", err)
			}
			_ = repo.CancelBilling(context.Background(), hold.ID)
		})
	}
}

func TestQuotaPaidResultReadsRemainAvailableAfterFundsAreSpent(t *testing.T) {
	engine, repo, key := quotaHTTPFixture(t, 0.05, 0, func(http.ResponseWriter, *http.Request) { t.Error("paid upstream must not run") })
	key.Budget = 0.05
	if err := repo.UpdateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	if err := repo.SettleUsageCharge(context.Background(), &storage.UsageLogRecord{APIKey: key.Key, TenantID: key.TenantID, BillingID: "already-paid", Cost: 0.05, TotalTokens: 1}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/v1beta/batches/job", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1beta/batches/job", nil))
	if w.Code != 200 {
		t.Fatalf("paid result was blocked by exhausted funds: %d %s", w.Code, w.Body.String())
	}
	paid := quotaPost(engine, `{"model":"quota-model","messages":[{"role":"user","content":"hi"}]}`)
	if paid.Code != http.StatusPaymentRequired {
		t.Fatalf("new spending was admitted: %d", paid.Code)
	}
}

func TestQuotaAgentRequiresBoundedFlatFee(t *testing.T) {
	engine, repo, _ := quotaHTTPFixture(t, 1, 0, func(http.ResponseWriter, *http.Request) { t.Error("unexpected provider call") })
	engine.POST("/test/agent", func(c *gin.Context) {
		if requireFlatFeeAgentQuota(c, "quota-model") {
			c.Status(200)
		}
	})
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test/agent", nil))
		return w
	}
	if w := call(); w.Code != 422 {
		t.Fatalf("unbounded agent was admitted: %d %s", w.Code, w.Body.String())
	}
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{Model: "quota-model", GroupName: "default", FixedPrice: 0.1, Currency: "CNY"}); err != nil {
		t.Fatal(err)
	}
	if err := billing.GlobalEngine.ReloadPrices(); err != nil {
		t.Fatal(err)
	}
	if w := call(); w.Code != 200 {
		t.Fatalf("funded flat-fee agent rejected: %d %s", w.Code, w.Body.String())
	}
}
