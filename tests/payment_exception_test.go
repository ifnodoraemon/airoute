package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPaymentException_StripeErrorDoesNotFallbackToSandbox verifies that when
// Stripe responds with an error (HTTP 400/500/network), the gateway immediately
// aborts with HTTP 502 and never generates a simulated sandbox checkout or credits balance.
func TestPaymentException_StripeErrorDoesNotFallbackToSandbox(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	user := &storage.UserRecord{
		Username:     "customer1",
		PasswordHash: "hash",
		Role:         "user",
		Status:       "active",
		Balance:      10.0,
	}
	require.NoError(t, repo.CreateUser(user))
	token, _ := tokenForTestUser("customer1", "user", time.Hour)

	// Spin up isolated mock payment server that returns an HTTP 500 error
	mockStripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Stripe API gateway down"}}`))
	}))
	defer mockStripe.Close()

	t.Setenv("STRIPE_API_KEY", "sk_test_synthetic_key")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_synthetic_test")
	t.Setenv("STRIPE_API_BASE", mockStripe.URL)

	body, _ := json.Marshal(map[string]interface{}{
		"amount":   50.0,
		"currency": "CNY",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/wallet/recharge/stripe/session", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Must fail with Bad Gateway (502), NOT 200 OK
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "支付服务通道返回错误")
	assert.Contains(t, w.Body.String(), "兑换券")

	// Must not contain any sandbox/simulation mode or checkout URL
	assert.NotContains(t, w.Body.String(), "sandbox_simulation")
	assert.NotContains(t, w.Body.String(), "checkout_url")

	// User balance must remain unchanged
	u, err := repo.GetUserByUsername("customer1")
	require.NoError(t, err)
	assert.Equal(t, 10.0, u.Balance)
}

// TestPaymentException_DBWriteFailureDoesNotLeakCheckoutURL verifies that if saving
// the order record to the database fails, the handler aborts with HTTP 500 and
// never hands over the payment checkout link to the customer.
func TestPaymentException_DBWriteFailureDoesNotLeakCheckoutURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	user := &storage.UserRecord{
		Username:     "customer2",
		PasswordHash: "hash",
		Role:         "user",
		Status:       "active",
		Balance:      20.0,
	}
	require.NoError(t, repo.CreateUser(user))
	token, _ := tokenForTestUser("customer2", "user", time.Hour)

	// Spin up isolated mock Stripe server that returns a valid checkout URL
	mockStripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cs_test_mock_12345","url":"https://checkout.stripe.com/pay/cs_test_mock_12345"}`))
	}))
	defer mockStripe.Close()

	t.Setenv("STRIPE_API_KEY", "sk_test_synthetic_key")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_synthetic_test")
	t.Setenv("STRIPE_API_BASE", mockStripe.URL)

	// Simulate database write failure by dropping the recharge_orders table
	_, err = db.Exec("DROP TABLE recharge_orders")
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]interface{}{
		"amount":   100.0,
		"currency": "CNY",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/wallet/recharge/stripe/session", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Must fail with 500 Internal Server Error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "创建充值订单记录失败")

	// Crucial: The customer MUST NOT receive the Stripe checkout URL when DB persistence fails!
	assert.NotContains(t, w.Body.String(), "https://checkout.stripe.com/pay/cs_test_mock_12345")
	assert.NotContains(t, w.Body.String(), "checkout_url")
}

// TestPaymentException_UnconfiguredGatewayPromptsRedemption verifies that when
// no payment gateway is configured, online checkout returns HTTP 503 instructing
// the user to redeem gift cards, rather than entering sandbox mode.
func TestPaymentException_UnconfiguredGatewayPromptsRedemption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	user := &storage.UserRecord{
		Username:     "customer3",
		PasswordHash: "hash",
		Role:         "user",
		Status:       "active",
		Balance:      0.0,
	}
	require.NoError(t, repo.CreateUser(user))
	token, _ := tokenForTestUser("customer3", "user", time.Hour)

	_ = os.Unsetenv("STRIPE_API_KEY")

	body, _ := json.Marshal(map[string]interface{}{
		"amount": 30.0,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/wallet/recharge/stripe/session", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "兑换券")
	assert.NotContains(t, w.Body.String(), "sandbox_simulation")
	assert.NotContains(t, w.Body.String(), "checkout_url")
}
