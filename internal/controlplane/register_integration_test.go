package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRegistrationStopsMintingAccountsAfterRepeatedAttempts is the A05
// regression at the handler level: with open registration and a trial balance,
// an unauthenticated caller must not be able to loop the endpoint and mint an
// account + trial credit per request.
func TestRegistrationStopsMintingAccountsAfterRepeatedAttempts(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "5")
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

	handler, repo := newSecurityTestHandler(t)
	gin.SetMode(gin.TestMode)

	remoteAddr := "203.0.113.80:1234"
	minted := 0
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"username":"bulk-farm-%d","email":"bulk-farm-%d@example.com","password":"password123"}`, i, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// Rotating the forwarding header must not create a fresh bucket.
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		req.RemoteAddr = remoteAddr
		c.Request = req
		handler.Register(c)

		switch w.Code {
		case http.StatusOK:
			minted++
		case http.StatusTooManyRequests:
			if minted == 0 {
				t.Fatal("registration was throttled before any account could be created")
			}
			if minted > 5 {
				t.Fatalf("%d accounts were minted before throttling", minted)
			}
			// The throttled attempt must not have created the account.
			if user, _ := repo.GetUserByUsername(fmt.Sprintf("bulk-farm-%d", i)); user != nil {
				t.Fatalf("throttled attempt %d still created an account", i)
			}
			return
		default:
			t.Fatalf("registration %d = %d: %s", i, w.Code, w.Body.String())
		}
	}
	t.Fatalf("registration minted %d accounts without ever throttling", minted)
}
