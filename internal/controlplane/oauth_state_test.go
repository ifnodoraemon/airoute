package controlplane

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// signedOAuthStateForTest builds a state with an arbitrary payload so tests can
// exercise expiry and provider-mismatch paths without waiting.
func signedOAuthStateForTest(payload string) string {
	mac := hmac.New(sha256.New, oauthStateKey())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(mac.Sum(nil))
}

func TestOAuthStateRoundTrip(t *testing.T) {
	state, err := newOAuthState("github")
	if err != nil {
		t.Fatalf("newOAuthState: %v", err)
	}
	if err := verifyOAuthState(state, "github"); err != nil {
		t.Fatalf("expected state to verify, got %v", err)
	}
	// A state issued for one provider must not authorize another.
	if err := verifyOAuthState(state, "google"); err == nil {
		t.Fatal("expected provider mismatch to be rejected")
	}
	// Tampering with the signature or the payload must fail.
	if err := verifyOAuthState(state+"0", "github"); err == nil {
		t.Fatal("expected a tampered signature to be rejected")
	}
	if err := verifyOAuthState("not-a-state", "github"); err == nil {
		t.Fatal("expected a malformed state to be rejected")
	}
	if err := verifyOAuthState("", "github"); err == nil {
		t.Fatal("expected an empty state to be rejected")
	}
}

func TestOAuthStateExpires(t *testing.T) {
	payload := fmt.Sprintf("github|%d|%s", time.Now().Add(-2*oauthStateTTL).Unix(), "nonce")
	if err := verifyOAuthState(signedOAuthStateForTest(payload), "github"); err == nil {
		t.Fatal("expected an expired state to be rejected")
	}
}

func TestBindOAuthStateRequiresMatchingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state, err := newOAuthState("github")
	if err != nil {
		t.Fatalf("newOAuthState: %v", err)
	}

	// No cookie: the signed state alone is not enough (it would not stop login
	// CSRF from an attacker-initiated flow).
	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	c1.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/callback", nil)
	if err := bindOAuthState(c1, state, "github"); err == nil {
		t.Fatal("expected a missing state cookie to be rejected")
	}

	// Same-browser flow: the cookie matches the returned state.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/callback", nil)
	req.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: state})
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = req
	if err := bindOAuthState(c2, state, "github"); err != nil {
		t.Fatalf("expected a matching cookie to be accepted, got %v", err)
	}

	// A different cookie value is the CSRF case and must fail.
	other, err := newOAuthState("github")
	if err != nil {
		t.Fatalf("newOAuthState: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/callback", nil)
	req2.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: other})
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = req2
	if err := bindOAuthState(c3, state, "github"); err == nil {
		t.Fatal("expected a mismatched state cookie to be rejected")
	}
}

func TestOAuthStateUsesCurrentClusterSecret(t *testing.T) {
	old := append([]byte(nil), getAdminSecret()...)
	t.Cleanup(func() { SetAdminSecret(old) })
	SetAdminSecret([]byte("initial-node-secret"))
	before, _ := newOAuthState("google")
	SetAdminSecret([]byte("persisted-shared-cluster-secret"))
	if verifyOAuthState(before, "google") == nil {
		t.Fatal("state signed by old node secret accepted")
	}
	after, _ := newOAuthState("google")
	if err := verifyOAuthState(after, "google"); err != nil {
		t.Fatal(err)
	}
}
