package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func quotaHTTPFixture(t *testing.T, balance, fixed float64, upstream http.HandlerFunc) (*gin.Engine, *storage.Repository, *storage.APIKeyRecord) {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "quota-model", GroupName: "default", PromptPrice: 1, CompletionPrice: 10, FixedPrice: fixed, Currency: "CNY"})
	user := &storage.UserRecord{Username: "quota-http-user", PasswordHash: "synthetic", Role: "user", Status: "active", Balance: balance}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &storage.APIKeyRecord{Key: "sk-quota-http-synthetic", UserID: user.ID, TenantID: user.Username, Status: "active"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	ready := middleware.DataPlaneReady()
	middleware.SetDataPlaneReady(true)
	t.Cleanup(func() { middleware.SetDataPlaneReady(ready) })
	storage.InitBillingSettlement(repo)
	t.Cleanup(func() { storage.InitBillingSettlement(nil) })
	engine := gin.New()
	engine.Use(middleware.SessionKeyAuthMiddleware(func(*gin.Context) (*model.APIKeyConfig, *middleware.AccessDenial) {
		stored, err := repo.GetAPIKeyByKey(key.Key)
		if err != nil {
			return nil, middleware.BalanceUnknownDenial()
		}
		cfg := stored.ToConfig()
		return &cfg, nil
	}))
	handler := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "quota-upstream", Type: model.ProviderOpenAI, BaseURL: server.URL, Models: []string{"quota-model"}, Status: "active", Weight: 1, Protocols: []string{"openai_chat"}}}))
	engine.POST("/v1/chat/completions", handler.HandleChatCompletions)
	return engine, repo, key
}

func quotaPost(engine *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	return w
}

func TestQuotaAdmissionRejectsBeforeCallingUpstream(t *testing.T) {
	var calls atomic.Int64
	engine, repo, key := quotaHTTPFixture(t, 1, 0, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(500) })
	w := quotaPost(engine, `{"model":"quota-model","messages":[{"role":"user","content":"hi"}],"max_tokens":500000}`)
	if w.Code != http.StatusPaymentRequired || calls.Load() != 0 {
		t.Fatalf("unfunded request dispatched: status=%d calls=%d body=%s", w.Code, calls.Load(), w.Body.String())
	}
	hold, err := repo.ReserveBilling(context.Background(), key.Key, storage.ToUnits(1), 64)
	if err != nil {
		t.Fatalf("failed request leaked its hold: %v", err)
	}
	_ = repo.CancelBilling(context.Background(), hold.ID)
}

func TestQuotaAdmissionBoundsDefaultOnActualUpstreamWire(t *testing.T) {
	var received atomic.Int64
	engine, _, _ := quotaHTTPFixture(t, 100, 0, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Max int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received.Store(int64(body.Max))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"quota-response","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	})
	w := quotaPost(engine, `{"model":"quota-model","messages":[{"role":"user","content":"hi"}]}`)
	if w.Code != 200 || received.Load() != defaultPrepaidCompletionTokens {
		t.Fatalf("wire cap=%d status=%d body=%s", received.Load(), w.Code, w.Body.String())
	}
}

func TestQuotaAdmissionTimeoutDoesNotMakeFundsReusable(t *testing.T) {
	t.Setenv("GATEWAY_SETTLEMENT_WAIT_MS", "1")
	t.Setenv("GATEWAY_SETTLEMENT_LINGER_MS", "60000")
	engine, repo, key := quotaHTTPFixture(t, 0.10, 0, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"pending-response","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":5000,"total_tokens":5001}}`)
	})
	storage.GlobalAsyncLogger.Stop()
	storage.GlobalAsyncLogger = nil
	batcher := storage.InitSettlementBatcher(repo)
	t.Cleanup(batcher.Stop)
	body := `{"model":"quota-model","messages":[{"role":"user","content":"hi"}],"max_tokens":5000}`
	first := quotaPost(engine, body)
	if first.Code != 200 {
		t.Fatalf("first request: %d %s", first.Code, first.Body.String())
	}
	user, _ := repo.GetUserByID(key.UserID)
	if user.BalanceUnits != storage.ToUnits(0.10) {
		t.Fatalf("expected deferred settlement, balance=%d", user.BalanceUnits)
	}
	second := quotaPost(engine, body)
	if second.Code != http.StatusPaymentRequired {
		t.Fatalf("reused pending funds: status=%d body=%s", second.Code, second.Body.String())
	}
	batcher.Stop()
	user, _ = repo.GetUserByID(key.UserID)
	if user.BalanceUnits != storage.ToUnits(0.049999) {
		t.Fatalf("settled balance=%d", user.BalanceUnits)
	}
}

func TestQuotaNativeOutputCapsPreserveProtocolAndExtensions(t *testing.T) {
	for _, tc := range []struct{ protocol, body, field string }{
		{model.NativeChat, `{"max_completion_tokens":12000,"vendor_extra":9007199254740993}`, "max_completion_tokens"},
		{model.NativeResponses, `{"max_output_tokens":12000,"vendor_extra":9007199254740993}`, "max_output_tokens"},
		{model.NativeMessages, `{"max_tokens":12000,"vendor_extra":9007199254740993}`, "max_tokens"},
		{model.NativeGemini, `{"generationConfig":{"maxOutputTokens":12000},"vendor_extra":9007199254740993}`, "maxOutputTokens"},
		{model.NativeGeminiInteractions, `{"generation_config":{"max_output_tokens":12000},"vendor_extra":9007199254740993}`, "max_output_tokens"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			req := &model.ChatCompletionRequest{Native: &model.NativeRequest{Protocol: tc.protocol, Body: []byte(tc.body)}}
			if err := boundPrepaidCompletion(req); err != nil {
				t.Fatal(err)
			}
			if *req.MaxTokens != 12000 || !strings.Contains(string(req.Native.Body), `9007199254740993`) || !strings.Contains(string(req.Native.Body), tc.field) {
				t.Fatalf("cap or native fields changed: %s", req.Native.Body)
			}
		})
	}
}
