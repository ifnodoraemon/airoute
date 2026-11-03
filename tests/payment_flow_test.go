package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

// Exercise the real authentication, routes, synchronizer, inference admission,
// and ledger using only controlled HTTP providers and an in-memory database.
func TestPaymentHTTPFlowRefundDebtStopsInference(t *testing.T) {
	var mu sync.Mutex
	var paid, refunded bool
	var number string
	expires := time.Now().Add(23 * time.Hour).Unix()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Stripe-Version") != controlplane.StripeAPIVersion {
			w.WriteHeader(400)
			return
		}
		session := func() map[string]any {
			s := map[string]any{"id": "cs_flow", "client_reference_id": number, "mode": "payment", "livemode": false, "status": "open", "payment_status": "unpaid", "amount_total": 5000, "currency": "cny", "expires_at": expires, "url": "https://checkout.stripe.com/pay/cs_flow"}
			if paid {
				s["status"] = "complete"
				s["payment_status"] = "paid"
				s["payment_intent"] = "pi_flow"
				s["url"] = ""
			}
			return s
		}
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			_ = r.ParseForm()
			number = r.Form.Get("client_reference_id")
			if r.Header.Get("Idempotency-Key") == "" || number == "" {
				w.WriteHeader(400)
				return
			}
			_ = json.NewEncoder(w).Encode(session())
		case "/v1/checkout/sessions/cs_flow":
			_ = json.NewEncoder(w).Encode(session())
		case "/v1/refunds":
			list := []map[string]any{}
			if refunded {
				list = append(list, map[string]any{"id": "re_flow", "amount": 5000, "currency": "cny", "payment_intent": "pi_flow", "status": "succeeded"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": list, "has_more": false})
		case "/v1/disputes":
			fmt.Fprint(w, `{"data":[],"has_more":false}`)
		case "/v1/refunds/re_flow":
			fmt.Fprint(w, `{"id":"re_flow","payment_intent":"pi_flow"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	t.Setenv("STRIPE_MODE", "test")
	t.Setenv("STRIPE_API_KEY", "sk_test_flow")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_flow")
	t.Setenv("STRIPE_API_BASE", provider.URL)
	t.Setenv("PUBLIC_URL", "https://wallet.example.invalid")
	t.Setenv("STRIPE_SUCCESS_URL", "")
	t.Setenv("STRIPE_MIN_RECHARGE_CNY", "10")
	app := newClosureApplication(t)
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat-payment-flow","object":"chat.completion","model":"remote-chat","choices":[{"index":0,"message":{"role":"assistant","content":"paid answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":0,"total_tokens":1000}}`)
	}))
	defer upstream.Close()
	channel := app.createChannel("payment-flow", upstream.URL, 1)
	app.call("POST", "/api/v1/admin/models/routes", app.admin, fmt.Sprintf(`{"model":"payment-flow","provider_updates":[{"channel_id":%d,"priority":1,"weight":10,"mapped_model":"remote-chat"}],"prompt_price":1000,"completion_price":1000,"currency":"CNY"}`, channel), 200)
	created := app.call("POST", "/api/v1/admin/users", app.admin, `{"username":"payment-customer","password":"payment-customer-password","balance":0}`, 200)
	apiKey := closureData(t, created)["api_key"].(string)
	login := app.call("POST", "/api/v1/auth/login", "", `{"username":"payment-customer","password":"payment-customer-password"}`, 200)
	userToken := closureData(t, login)["token"].(string)
	chat := `{"model":"payment-flow","messages":[{"role":"user","content":"hello"}],"max_tokens":20}`
	app.call("POST", "/v1/chat/completions", apiKey, chat, 402)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/user/wallet/recharge/stripe/session", strings.NewReader(`{"amount":50}`))
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "payment-flow-checkout")
	app.engine.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "checkout.stripe.com")
	notify := func(id, kind, object string) {
		body, _ := json.Marshal(map[string]any{"id": id, "type": kind, "api_version": controlplane.StripeAPIVersion, "livemode": false, "data": map[string]any{"object": map[string]any{"id": object}}})
		ts := fmt.Sprint(time.Now().Unix())
		mac := hmac.New(sha256.New, []byte("whsec_flow"))
		_, _ = mac.Write(append([]byte(ts+"."), body...))
		req := httptest.NewRequest("POST", "/api/v1/public/stripe/webhook", strings.NewReader(string(body)))
		req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%s,v1=%x", ts, mac.Sum(nil)))
		w := httptest.NewRecorder()
		app.engine.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
	}
	mu.Lock()
	paid = true
	mu.Unlock()
	notify("evt_flow_paid", "checkout.session.completed", "cs_flow")
	u, err := app.repo.GetUserByUsername("payment-customer")
	require.NoError(t, err)
	require.Equal(t, 50.0, u.Balance)
	app.call("POST", "/v1/chat/completions", apiKey, chat, 200)
	require.Eventually(t, func() bool {
		u, err := app.repo.GetUserByUsername("payment-customer")
		return err == nil && u.BalanceUnits > 0 && u.BalanceUnits < storage.ToUnits(50)
	}, 3*time.Second, 10*time.Millisecond)
	u, err = app.repo.GetUserByUsername("payment-customer")
	require.NoError(t, err)
	spentBalanceUnits := u.BalanceUnits
	mu.Lock()
	refunded = true
	mu.Unlock()
	notify("evt_flow_refund", "refund.updated", "re_flow")
	u, err = app.repo.GetUserByUsername("payment-customer")
	require.NoError(t, err)
	require.Equal(t, spentBalanceUnits-storage.ToUnits(50), u.BalanceUnits)
	require.Negative(t, u.BalanceUnits, "refunding consumed credit must leave a debt")
	notify("evt_flow_paid", "checkout.session.completed", "cs_flow")
	app.call("POST", "/v1/chat/completions", apiKey, chat, 402)
	require.Equal(t, int64(1), upstreamCalls.Load())
	orders, err := app.repo.ListRechargeOrders("payment-customer")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	require.Equal(t, "refunded", orders[0].Status)
	app.call("POST", "/api/v1/user/wallet/recharge/sandbox", userToken, `{"amount":50}`, 404)
}
