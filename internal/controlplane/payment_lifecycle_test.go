package controlplane

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

type paymentProvider struct {
	mu                  sync.Mutex
	sessions            map[string]stripeSession
	keys                map[string]string
	refunds, disputes   []map[string]any
	postKeys            []string
	postCount, getCount int
	fail, loseResponse  bool
	amountOverride      int64
	liveOverride        bool
}

func (p *paymentProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Stripe-Version") != StripeAPIVersion {
		w.WriteHeader(400)
		return
	}
	if p.fail {
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"message":"controlled outage"}}`)
		return
	}
	var result any
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/checkout/sessions":
		p.postCount++
		p.postKeys = append(p.postKeys, r.Header.Get("Idempotency-Key"))
		_ = r.ParseForm()
		key := r.Header.Get("Idempotency-Key")
		id := p.keys[key]
		if id == "" {
			id = fmt.Sprintf("cs_payment_%d", len(p.sessions)+1)
			p.keys[key] = id
			amount, _ := strconv.ParseInt(r.Form.Get("line_items[0][price_data][unit_amount]"), 10, 64)
			expires, _ := strconv.ParseInt(r.Form.Get("expires_at"), 10, 64)
			live := false
			if p.liveOverride {
				live = true
			}
			if p.amountOverride != 0 {
				amount = p.amountOverride
			}
			p.sessions[id] = stripeSession{ID: id, Reference: r.Form.Get("client_reference_id"), Mode: "payment", LiveMode: &live, Status: "open", PaymentStatus: "unpaid", Amount: amount, Currency: "cny", URL: "https://checkout.stripe.com/pay/" + id, ExpiresAt: expires}
		}
		if p.loseResponse {
			p.loseResponse = false
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"message":"response lost after create"}}`)
			return
		}
		result = p.sessions[id]
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
		p.getCount++
		s, ok := p.sessions[strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/")]
		if !ok {
			w.WriteHeader(404)
			return
		}
		result = s
	case r.Method == "GET" && r.URL.Path == "/v1/checkout/sessions":
		list := []stripeSession{}
		for _, s := range p.sessions {
			pi := r.URL.Query().Get("payment_intent")
			if pi == "" || string(s.PaymentIntent) == pi {
				list = append(list, s)
			}
		}
		result = map[string]any{"data": list, "has_more": false}
	case r.URL.Path == "/v1/refunds":
		result = map[string]any{"data": p.refunds, "has_more": false}
	case r.URL.Path == "/v1/disputes":
		result = map[string]any{"data": p.disputes, "has_more": false}
	case r.URL.Path == "/v1/payment_intents/pi_payment":
		amount := int64(0)
		for _, s := range p.sessions {
			amount = s.Amount
		}
		result = map[string]any{"id": "pi_payment", "amount": amount, "currency": "cny", "livemode": false, "status": "canceled"}
	case strings.HasPrefix(r.URL.Path, "/v1/refunds/") || strings.HasPrefix(r.URL.Path, "/v1/disputes/") || strings.HasPrefix(r.URL.Path, "/v1/charges/"):
		result = map[string]any{"payment_intent": "pi_payment", "livemode": false}
	default:
		w.WriteHeader(404)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

type paymentFixture struct {
	h        *AdminHandler
	repo     *storage.Repository
	provider *paymentProvider
	t        *testing.T
}

func newPaymentFixture(t *testing.T) *paymentFixture {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	h, repo := regressionHandler(t)
	require.NoError(t, repo.CreateUser(&storage.UserRecord{Username: "regression-client", PasswordHash: "synthetic", Role: "user", Status: "active", Balance: 10}))
	p := &paymentProvider{sessions: map[string]stripeSession{}, keys: map[string]string{}}
	server := httptest.NewServer(p)
	t.Cleanup(server.Close)
	t.Setenv("STRIPE_MODE", "test")
	t.Setenv("STRIPE_API_KEY", "sk_test_controlled")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_controlled")
	t.Setenv("STRIPE_API_BASE", server.URL)
	t.Setenv("PUBLIC_URL", "https://wallet.example.invalid")
	t.Setenv("STRIPE_SUCCESS_URL", "")
	t.Setenv("STRIPE_MIN_RECHARGE_CNY", "10")
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(&config.Config{})
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	return &paymentFixture{h: h, repo: repo, provider: p, t: t}
}

func (f *paymentFixture) create(key string, amount float64) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/checkout", strings.NewReader(fmt.Sprintf(`{"amount":%.9f}`, amount)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Idempotency-Key", key)
	c.Set("admin_claims", &AdminClaims{Username: "regression-client", Role: "user"})
	f.h.CreateStripeRechargeSession(c)
	return w
}

func (f *paymentFixture) order() *storage.RechargeOrderRecord {
	orders, err := f.repo.ListRechargeOrders("regression-client")
	require.NoError(f.t, err)
	require.Len(f.t, orders, 1)
	return orders[0]
}
func (f *paymentFixture) balance() float64 {
	u, err := f.repo.GetUserByUsername("regression-client")
	require.NoError(f.t, err)
	return u.Balance
}
func (f *paymentFixture) paid() {
	f.provider.mu.Lock()
	defer f.provider.mu.Unlock()
	for id, s := range f.provider.sessions {
		s.PaymentStatus = "paid"
		s.Status = "complete"
		s.PaymentIntent = "pi_payment"
		s.URL = ""
		f.provider.sessions[id] = s
	}
}
func (f *paymentFixture) due() {
	_, err := f.repo.GetDB().Exec(`UPDATE recharge_orders SET next_reconcile_at=0`)
	require.NoError(f.t, err)
}
func (f *paymentFixture) poll(number, user string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/orders/"+number, nil)
	c.Params = gin.Params{{Key: "order_no", Value: number}}
	c.Set("admin_claims", &AdminClaims{Username: user, Role: "user"})
	f.h.GetUserRechargeOrder(c)
	return w
}
func (f *paymentFixture) event(id, kind, object string, live bool, version string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"id": id, "type": kind, "api_version": version, "livemode": live, "data": map[string]any{"object": map[string]any{"id": object}}})
	ts := fmt.Sprint(time.Now().Unix())
	mac := hmac.New(sha256.New, []byte("whsec_controlled"))
	_, _ = mac.Write(append([]byte(ts+"."), body...))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook", strings.NewReader(string(body)))
	c.Request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%s,v1=%x", ts, mac.Sum(nil)))
	f.h.StripeWebhook(c)
	return w
}
func (f *paymentFixture) sync() {
	f.due()
	cfg, err := loadStripeConfig()
	require.NoError(f.t, err)
	_, err = f.h.syncStripeOrder(context.Background(), f.order().OrderNo, cfg, true, "")
	require.NoError(f.t, err)
}

func TestPaymentCreateIsIdempotentAndStoresIntentFirst(t *testing.T) {
	f := newPaymentFixture(t)
	w := f.create("same-payment-intent", 19.99)
	require.Equal(t, 200, w.Code, w.Body.String())
	o := f.order()
	require.Equal(t, int64(1999), o.Cents())
	require.Equal(t, "pending", o.Status)
	require.Equal(t, 10.0, f.balance())
	w = f.create("same-payment-intent", 19.99)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, o.OrderNo, f.order().OrderNo)
	require.Equal(t, 1, f.provider.postCount)
	require.Equal(t, "airoute-checkout-"+o.OrderNo, f.provider.postKeys[0])
	w = f.create("same-payment-intent", 50)
	require.Equal(t, 409, w.Code)
	require.Equal(t, 1, f.provider.postCount)
	_, err := f.repo.GetDB().Exec(`DROP TABLE recharge_orders`)
	require.NoError(t, err)
	w = f.create("different-payment-intent", 50)
	require.Equal(t, 500, w.Code)
	require.Equal(t, 1, f.provider.postCount, "must not contact provider when local insert fails")
}

func TestPaymentAmbiguousCreationAndWriteFailureRecoverSameSession(t *testing.T) {
	for _, failure := range []string{"lost_response", "database_update"} {
		t.Run(failure, func(t *testing.T) {
			f := newPaymentFixture(t)
			if failure == "lost_response" {
				f.provider.loseResponse = true
			} else {
				_, err := f.repo.GetDB().Exec(`CREATE TRIGGER reject_session BEFORE UPDATE OF stripe_session_id ON recharge_orders BEGIN SELECT RAISE(FAIL,'controlled write failure'); END`)
				require.NoError(t, err)
			}
			w := f.create("recover-same-intent", 50)
			require.Equal(t, 503, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "checkout.stripe.com")
			require.Len(t, f.provider.sessions, 1)
			require.Empty(t, f.order().StripeSessionID)
			if failure == "database_update" {
				_, err := f.repo.GetDB().Exec(`DROP TRIGGER reject_session`)
				require.NoError(t, err)
			}
			w = f.create("recover-same-intent", 50)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Len(t, f.provider.sessions, 1)
			require.NotEmpty(t, f.order().StripeSessionID)
		})
	}
}

func TestPaymentPollAndBackgroundRecoverMissingNotifications(t *testing.T) {
	for _, source := range []string{"poll", "background"} {
		t.Run(source, func(t *testing.T) {
			f := newPaymentFixture(t)
			require.Equal(t, 200, f.create("missed-notification", 50).Code)
			f.paid()
			f.due()
			if source == "poll" {
				require.Equal(t, 200, f.poll(f.order().OrderNo, "regression-client").Code)
			} else {
				f.h.reconcileStripePayments(context.Background())
			}
			require.Equal(t, 60.0, f.balance())
			require.Equal(t, "paid", f.order().Status)
			f.h.reconcileStripePayments(context.Background())
			require.Equal(t, 60.0, f.balance())
		})
	}
}

func TestPaymentLateNotificationRecoversUnboundExpiredIntent(t *testing.T) {
	f := newPaymentFixture(t)
	require.Equal(t, 200, f.create("late-after-process-crash", 50).Code)
	f.paid()
	o := f.order()
	id := o.StripeSessionID
	f.provider.mu.Lock()
	s := f.provider.sessions[id]
	s.ExpiresAt = time.Now().Add(-time.Hour).Unix()
	f.provider.sessions[id] = s
	f.provider.mu.Unlock()
	_, err := f.repo.GetDB().Exec(`UPDATE recharge_orders SET stripe_session_id='',payment_intent_id='',expires_at=?,last_sync_error='' WHERE order_no=?`, s.ExpiresAt, o.OrderNo)
	require.NoError(t, err)
	w := f.event("evt_recovered_late", "checkout.session.completed", id, false, StripeAPIVersion)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 60.0, f.balance())
	require.Equal(t, 1, f.provider.postCount, "notification must use known session instead of recreating expired intent")
}

func TestPaymentAsyncFailureUsesCurrentIntentState(t *testing.T) {
	f := newPaymentFixture(t)
	require.Equal(t, 200, f.create("asynchronous-failure", 50).Code)
	id := f.order().StripeSessionID
	f.provider.mu.Lock()
	s := f.provider.sessions[id]
	s.Status = "complete"
	s.PaymentIntent = "pi_payment"
	f.provider.sessions[id] = s
	f.provider.mu.Unlock()
	w := f.event("evt_async_failed", "checkout.session.async_payment_failed", id, false, StripeAPIVersion)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "failed", f.order().Status)
	require.Equal(t, 10.0, f.balance())
}

func TestPaymentRefundDisputeNetAmountsAndDebt(t *testing.T) {
	f := newPaymentFixture(t)
	require.Equal(t, 200, f.create("refund-dispute-intent", 50).Code)
	f.paid()
	f.sync()
	require.Equal(t, 60.0, f.balance())
	refund := func(status string, amount int64) map[string]any {
		return map[string]any{"id": "re_payment", "amount": amount, "currency": "cny", "payment_intent": "pi_payment", "status": status}
	}
	dispute := func(status string) map[string]any {
		return map[string]any{"id": "du_payment", "amount": 5000, "currency": "cny", "payment_intent": "pi_payment", "status": status, "livemode": false}
	}
	f.provider.refunds = []map[string]any{refund("pending", 2000)}
	f.sync()
	require.Equal(t, 40.0, f.balance())
	require.Equal(t, "refund_pending", f.order().Status)
	f.provider.refunds = []map[string]any{refund("failed", 2000)}
	f.sync()
	require.Equal(t, 60.0, f.balance())
	f.provider.refunds = []map[string]any{refund("succeeded", 2000)}
	f.sync()
	f.sync()
	require.Equal(t, 40.0, f.balance())
	require.Equal(t, "partially_refunded", f.order().Status)
	f.provider.disputes = []map[string]any{dispute("needs_response")}
	f.sync()
	require.Equal(t, 10.0, f.balance(), "overlapping refund/dispute must not withdraw more than original credit")
	foreign := dispute("under_review")
	foreign["currency"] = "usd"
	foreign["amount"] = 600
	f.provider.disputes = []map[string]any{foreign}
	f.sync()
	require.Equal(t, 10.0, f.balance(), "foreign-currency disputes reserve principal without inventing an exchange rate")
	higher := dispute("under_review")
	higher["amount"] = 5500
	f.provider.disputes = []map[string]any{higher}
	f.sync()
	require.Equal(t, 10.0, f.balance(), "FX-adjusted disputes may exceed the original charge")
	f.provider.disputes = []map[string]any{dispute("won")}
	f.sync()
	require.Equal(t, 40.0, f.balance(), "a win releases only disputed funds, retaining successful refund")
	require.NoError(t, f.repo.UpdateUserBalance("regression-client", -35))
	f.provider.refunds = append(f.provider.refunds, map[string]any{"id": "re_remaining", "amount": 3000, "currency": "cny", "payment_intent": "pi_payment", "status": "succeeded"})
	f.sync()
	require.Equal(t, -25.0, f.balance(), "spent refundable credit must become debt")
	require.Equal(t, "refunded", f.order().Status)
	// An old completed notification reads current refunds and cannot re-credit.
	w := f.event("evt_late_completed", "checkout.session.completed", f.order().StripeSessionID, false, StripeAPIVersion)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, -25.0, f.balance())
}

func TestPaymentRefundEventBeforeCheckoutAndEventRecovery(t *testing.T) {
	f := newPaymentFixture(t)
	require.Equal(t, 200, f.create("refund-arrives-first", 50).Code)
	f.paid()
	f.provider.refunds = []map[string]any{{"id": "re_payment", "amount": 5000, "currency": "cny", "payment_intent": "pi_payment", "status": "succeeded"}}
	f.provider.fail = true
	w := f.event("evt_refund_first", "refund.updated", "re_payment", false, StripeAPIVersion)
	require.Equal(t, 503, w.Code)
	var pending int
	require.NoError(t, f.repo.GetDB().QueryRow(`SELECT COUNT(*) FROM stripe_events WHERE processed=0`).Scan(&pending))
	require.Equal(t, 1, pending)
	f.provider.fail = false
	_, err := f.repo.GetDB().Exec(`UPDATE stripe_events SET next_attempt_at=0`)
	require.NoError(t, err)
	f.h.reconcileStripePayments(context.Background())
	require.Equal(t, 10.0, f.balance())
	require.Equal(t, "refunded", f.order().Status)
	w = f.event("evt_refund_first", "refund.updated", "re_payment", false, StripeAPIVersion)
	require.Equal(t, 200, w.Code)
	require.Equal(t, 10.0, f.balance())
}

func TestPaymentRejectsModeVersionAmountAndForeignOrder(t *testing.T) {
	f := newPaymentFixture(t)
	require.Equal(t, 200, f.create("verified-payment-intent", 50).Code)
	for _, input := range []struct {
		live    bool
		version string
	}{{true, StripeAPIVersion}, {false, "2020-08-27"}} {
		require.Equal(t, 400, f.event("evt_bad_environment", "checkout.session.completed", f.order().StripeSessionID, input.live, input.version).Code)
	}
	f.paid()
	f.provider.mu.Lock()
	s := f.provider.sessions[f.order().StripeSessionID]
	s.Amount = 100
	s.Currency = "usd"
	f.provider.sessions[s.ID] = s
	f.provider.mu.Unlock()
	require.Equal(t, 503, f.event("evt_bad_amount", "checkout.session.completed", s.ID, false, StripeAPIVersion).Code)
	require.Equal(t, 10.0, f.balance())
	gets := f.provider.getCount
	require.Equal(t, 404, f.poll(f.order().OrderNo, "another-user").Code)
	require.Equal(t, gets, f.provider.getCount, "ownership check precedes provider calls")
}

func TestPaymentConfigurationAndMinimum(t *testing.T) {
	f := newPaymentFixture(t)
	require.Equal(t, 400, f.create("", 50).Code)
	require.Equal(t, 400, f.create("minimum-amount-intent", 0.01).Code)
	for _, amount := range []float64{0.001, 10.999, 1000000} {
		require.Equal(t, 400, f.create("invalid-amount-intent", amount).Code)
	}
	t.Setenv("STRIPE_SUCCESS_URL", "https://wallet.example.invalid/return?keep=yes")
	require.Equal(t, 200, f.create("redirect-payment-intent", 50).Code)
	o := f.order()
	require.Contains(t, o.SuccessURL, "order_no="+o.OrderNo)
	require.Contains(t, o.SuccessURL, "keep=yes")
	require.Contains(t, o.SuccessURL, "recharge=success")
	t.Setenv("STRIPE_MODE", "live")
	require.Error(t, f.h.ValidatePaymentConfiguration(), "test key is not live")
	t.Setenv("STRIPE_API_KEY", "sk_live_controlled")
	t.Setenv("STRIPE_API_BASE", "")
	require.Error(t, f.h.ValidatePaymentConfiguration(), "a test database cannot switch to live")
	t.Setenv("STRIPE_MODE", "test")
	t.Setenv("STRIPE_API_KEY", "sk_test_controlled")
	t.Setenv("STRIPE_SUCCESS_URL", "https://attacker.example/return")
	require.Error(t, f.h.ValidatePaymentConfiguration())
	t.Setenv("STRIPE_SUCCESS_URL", "")
	t.Setenv("PUBLIC_URL", "")
	require.Error(t, f.h.ValidatePaymentConfiguration())
	t.Setenv("STRIPE_API_KEY", "")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")
	t.Setenv("STRIPE_MODE", "live")
	require.Error(t, f.h.ValidatePaymentConfiguration(), "disabling payments must not bypass a database's environment binding")
}

func TestStripeSignatureVerification(t *testing.T) {
	body := []byte(`{"id":"evt_signature"}`)
	secret := "whsec_signature"
	ts := fmt.Sprint(time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(append([]byte(ts+"."), body...))
	header := fmt.Sprintf("t=%s,v1=deadbeef,v1=%x", ts, mac.Sum(nil))
	require.NoError(t, VerifyStripeWebhookSignature(body, header, secret, 5*time.Minute))
	require.Error(t, VerifyStripeWebhookSignature(append(body, ' '), header, secret, 5*time.Minute))
	require.Error(t, VerifyStripeWebhookSignature(body, header, "", 5*time.Minute))
	require.Error(t, VerifyStripeWebhookSignature(body, "", secret, 5*time.Minute))
	old := fmt.Sprint(time.Now().Add(-10 * time.Minute).Unix())
	require.Error(t, VerifyStripeWebhookSignature(body, "t="+old+",v1=deadbeef", secret, 5*time.Minute))
}
