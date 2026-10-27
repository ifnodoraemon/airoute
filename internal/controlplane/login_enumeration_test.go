package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// setupLoginEnumerationFixture creates one active account, one locked account and
// a Gin engine wired to the real Login handler.
func setupLoginEnumerationFixture(t *testing.T) (*AdminHandler, *storage.Repository, *gin.Engine) {
	t.Helper()
	handler, repo := newSecurityTestHandler(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/auth/login", handler.Login)

	activeHash, err := bcrypt.GenerateFromPassword([]byte("active-correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	lockedHash, err := bcrypt.GenerateFromPassword([]byte("locked-correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	for _, u := range []*storage.UserRecord{
		{Username: "enum-active", Email: "enum-active@example.com", PasswordHash: string(activeHash), Role: "user", Status: "active"},
		{Username: "enum-locked", Email: "enum-locked@example.com", PasswordHash: string(lockedHash), Role: "user", Status: "locked"},
		{Username: "enum-deleted", Email: "enum-deleted@example.com", PasswordHash: "", Role: "user", Status: "deleted"},
	} {
		if err := repo.CreateUser(u); err != nil {
			t.Fatalf("create %s: %v", u.Username, err)
		}
	}
	return handler, repo, engine
}

func postLogin(engine *gin.Engine, remoteAddr, username, password string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	engine.ServeHTTP(w, req)
	return w
}

// TestLoginResponsesAreIndistinguishableForUnknownAndWrongPassword is the A13
// regression for the account-state oracle: the same status and body must come
// back for an unknown account, a deleted account and a wrong password, and a
// locked account must not advertise its state to a caller who does not know the
// password.
func TestLoginResponsesAreIndistinguishableForUnknownAndWrongPassword(t *testing.T) {
	_, _, engine := setupLoginEnumerationFixture(t)

	cases := []struct {
		name     string
		username string
		password string
	}{
		{"unknown account", "enum-does-not-exist", "whatever"},
		{"deleted account", "enum-deleted", "whatever"},
		{"wrong password", "enum-active", "wrong-password"},
		{"locked account with wrong password", "enum-locked", "wrong-password"},
	}
	var wantStatus int
	var wantBody string
	for i, tc := range cases {
		// Distinct client addresses keep the shared lockout out of the way.
		w := postLogin(engine, fmt.Sprintf("203.0.113.%d:1000", 200+i), tc.username, tc.password)
		gotBody := strings.TrimSpace(w.Body.String())
		if i == 0 {
			wantStatus, wantBody = w.Code, gotBody
			if wantStatus != http.StatusUnauthorized {
				t.Fatalf("unknown account = %d, want 401", wantStatus)
			}
			continue
		}
		if w.Code != wantStatus || gotBody != wantBody {
			t.Fatalf("%s = %d %q, want %d %q (account state must not be distinguishable)",
				tc.name, w.Code, gotBody, wantStatus, wantBody)
		}
	}

	// The locked account still reports its state once the password is proven, so
	// a legitimate user gets a useful message.
	w := postLogin(engine, "203.0.113.250:1000", "enum-locked", "locked-correct-password")
	if w.Code != http.StatusForbidden {
		t.Fatalf("locked account with the correct password = %d, want 403 (%s)", w.Code, w.Body.String())
	}
}

// TestUnknownAccountPaysTheBcryptCost pins the timing equalization: the unknown
// account path must perform a real bcrypt comparison (against the dummy hash)
// rather than returning immediately.
func TestUnknownAccountPaysTheBcryptCost(t *testing.T) {
	_, _, engine := setupLoginEnumerationFixture(t)

	var unknownComparisons atomic.Int64
	var existingComparisons atomic.Int64
	original := bcryptCompare
	t.Cleanup(func() { bcryptCompare = original })
	bcryptCompare = func(hash, password []byte) error {
		if string(hash) == dummyPasswordHash {
			unknownComparisons.Add(1)
		} else {
			existingComparisons.Add(1)
		}
		return bcrypt.CompareHashAndPassword(hash, password)
	}

	if w := postLogin(engine, "203.0.113.10:1000", "no-such-account-xyz", "whatever"); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown account = %d, want 401", w.Code)
	}
	if unknownComparisons.Load() != 1 {
		t.Fatalf("unknown account performed %d bcrypt comparisons, want exactly 1 dummy comparison",
			unknownComparisons.Load())
	}
	if existingComparisons.Load() != 0 {
		t.Fatalf("unknown account unexpectedly compared a stored hash")
	}

	if w := postLogin(engine, "203.0.113.11:1000", "enum-active", "wrong-password"); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", w.Code)
	}
	if existingComparisons.Load() != 1 {
		t.Fatalf("existing account compared its hash %d times, want 1", existingComparisons.Load())
	}

	// A deleted account must pay the dummy cost too.
	if w := postLogin(engine, "203.0.113.12:1000", "enum-deleted", "whatever"); w.Code != http.StatusUnauthorized {
		t.Fatalf("deleted account = %d, want 401", w.Code)
	}
	if unknownComparisons.Load() != 2 {
		t.Fatalf("deleted account did not pay the dummy comparison (count=%d)", unknownComparisons.Load())
	}
}

// TestLoginTimingEnvelopeForUnknownAccount is the coarse end-to-end check: the
// unknown-account response must cost roughly as much as a real verification.
// A production-cost bcrypt hash takes far longer than the 5ms floor asserted here.
func TestLoginTimingEnvelopeForUnknownAccount(t *testing.T) {
	handler, repo := newSecurityTestHandler(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/auth/login", handler.Login)
	hash, err := bcrypt.GenerateFromPassword([]byte("timing-correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := repo.CreateUser(&storage.UserRecord{
		Username: "timing-user", Email: "timing@example.com", PasswordHash: string(hash), Role: "user", Status: "active",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	measure := func(username string) time.Duration {
		start := time.Now()
		if w := postLogin(engine, "203.0.113.77:1000", username, "wrong-password"); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s = %d, want 401", username, w.Code)
		}
		return time.Since(start)
	}
	known := measure("timing-user")
	unknown := measure("timing-unknown-user")

	// Before the fix the unknown path was ~2ms (no bcrypt) against ~280ms for a
	// real account. Any value in the same order of magnitude is enough here; the
	// exact equality is covered by the comparison-count test above.
	if unknown < known/3 {
		t.Fatalf("unknown account responded in %v vs %v for a known account: the timing oracle is still open", unknown, known)
	}
	if unknown < 5*time.Millisecond {
		t.Fatalf("unknown account responded in %v: it did not pay a bcrypt comparison", unknown)
	}
}
