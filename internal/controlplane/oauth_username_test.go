package controlplane

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestOAuthUsernameBase pins the preferred name and its sanitization.
func TestOAuthUsernameBase(t *testing.T) {
	if got := oauthUsernameBase("github", "123456"); got != "gh_123456" {
		t.Fatalf("github base = %q", got)
	}
	if got := oauthUsernameBase("google", "109876543210"); got != "gg_109876543210" {
		t.Fatalf("google base = %q", got)
	}
	// Provider-controlled characters that are not safe in a username are dropped.
	if got := oauthUsernameBase("google", "a b/c:d"); got != "gg_abcd" {
		t.Fatalf("sanitized base = %q", got)
	}
	// An unusable subject still produces a stable, distinct name.
	empty := oauthUsernameBase("github", "")
	if !strings.HasPrefix(empty, "gh_") || len(empty) <= len("gh_") {
		t.Fatalf("empty subject base = %q", empty)
	}
	if empty != oauthUsernameBase("github", "") {
		t.Fatal("the fallback name must be deterministic for the same identity")
	}
}

// TestAllocateOAuthUsernameAvoidsSquattedNames is the A17 regression: a local
// registration of gh_<id> must not make that identity's first login fail. The
// allocator resolves the collision with a suffixed name instead of erroring.
func TestAllocateOAuthUsernameAvoidsSquattedNames(t *testing.T) {
	_, repository := newSecurityTestHandler(t)

	// Someone registered the obvious name locally before the GitHub user signed in.
	if err := repository.CreateUser(&storage.UserRecord{
		Username: "gh_123", Email: "squatter@example.com", PasswordHash: "x", Role: "user", Status: "active",
	}); err != nil {
		t.Fatalf("create squatter: %v", err)
	}

	allocated, err := allocateOAuthUsername(repository, "github", "123", "")
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if allocated == "gh_123" {
		t.Fatal("allocated the squatted name")
	}
	if !strings.HasPrefix(allocated, "gh_123_") {
		t.Fatalf("expected a suffixed variant of the preferred name, got %q", allocated)
	}
	if existing, err := repository.GetUserByUsername(allocated); !errors.Is(err, sql.ErrNoRows) || existing != nil {
		t.Fatalf("allocated name is not free: existing=%v err=%v", existing, err)
	}
}

// TestAllocateOAuthUsernameIsUniqueUnderRepeatedUse ensures repeated allocation
// for the same identity never returns the same free name twice.
func TestAllocateOAuthUsernameIsUniqueUnderRepeatedUse(t *testing.T) {
	_, repository := newSecurityTestHandler(t)
	seen := make(map[string]bool)
	for i := 0; i < 5; i++ {
		name, err := allocateOAuthUsername(repository, "google", "987654321", "")
		if err != nil {
			t.Fatalf("allocate %d: %v", i, err)
		}
		if seen[name] {
			t.Fatalf("allocator returned %q twice", name)
		}
		seen[name] = true
		if err := repository.CreateUser(&storage.UserRecord{
			Username: name, Email: name + "@example.com", PasswordHash: "x", Role: "user", Status: "active",
		}); err != nil {
			t.Fatalf("claim %q: %v", name, err)
		}
	}
}

// TestAllocateOAuthUsernameKeepsAnExplicitFreeName covers the dev/simulated path,
// where the caller asks for a specific name.
func TestAllocateOAuthUsernameKeepsAnExplicitFreeName(t *testing.T) {
	_, repository := newSecurityTestHandler(t)
	name, err := allocateOAuthUsername(repository, "github", "42", "demo-requested")
	if err != nil || name != "demo-requested" {
		t.Fatalf("allocate = %q, %v; want the requested free name", name, err)
	}
}

// TestRegistrationRejectsReservedOAuthUsernames stops the squatting vector at the
// door instead of relying on the allocator to dodge it.
func TestRegistrationRejectsReservedOAuthUsernames(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "0")

	handler, repo := newSecurityTestHandler(t)
	gin.SetMode(gin.TestMode)
	for _, username := range []string{"gh_123", "GG_456", "gh_taken"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := `{"username":"` + username + `","email":"` + strings.ToLower(username) + `@example.com","password":"password123"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.90:1000"
		c.Request = req
		handler.Register(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("registering %q = %d, want 400 (%s)", username, w.Code, w.Body.String())
		}
		if user, _ := repo.GetUserByUsername(username); user != nil {
			t.Fatalf("reserved username %q was created", username)
		}
	}

	// A normal name is unaffected.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"username":"ordinary-user","email":"ordinary@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.91:1000"
	c.Request = req
	handler.Register(c)
	if w.Code != http.StatusOK {
		t.Fatalf("ordinary registration = %d: %s", w.Code, w.Body.String())
	}
}
