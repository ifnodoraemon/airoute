package controlplane

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// seedRegistrationUser creates an account the registration handler must see as
// already taken.
func seedRegistrationUser(t *testing.T, repo *storage.Repository, username, email string) {
	t.Helper()
	user := &storage.UserRecord{
		Username: username, Email: email, PasswordHash: "synthetic", Role: "user", Status: "active",
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
}

// #17: a duplicate-username refusal used to happen after VerifyCode consumed the
// emailed code, so the obvious retry (fix the username, keep the code) always
// failed and the user had to request a new mail. The uniqueness checks now run
// first, so the same code still works for the corrected attempt.
func TestRegisterKeepsVerificationCodeAfterDuplicateUsername(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "true")
	t.Setenv("INITIAL_USER_BALANCE", "5")
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

	handler, repo := newSecurityTestHandler(t)
	seedRegistrationUser(t, repo, "squatted-name", "squatted@example.invalid")

	const code = "246810"
	if err := repo.SaveVerificationCode("fresh@example.invalid", code, "register", 10*time.Minute); err != nil {
		t.Fatalf("save verification code: %v", err)
	}

	// The first attempt is refused on the username, before the code is touched.
	first := registrationRequest(t, handler, "203.0.113.61:1000", "squatted-name", "fresh@example.invalid", code)
	if first.Code != http.StatusBadRequest {
		t.Fatalf("duplicate-username registration = %d, want 400: %s", first.Code, first.Body.String())
	}
	if !strings.Contains(first.Body.String(), "该用户名已被注册") {
		t.Fatalf("message = %s, want the duplicate-username refusal", first.Body.String())
	}
	if user, _ := repo.GetUserByUsername("squatted-name"); user == nil {
		t.Fatal("fixture user disappeared")
	}

	// The retry keeps the code and the same address, and must succeed.
	second := registrationRequest(t, handler, "203.0.113.61:1000", "fresh-name", "fresh@example.invalid", code)
	if second.Code != http.StatusOK {
		t.Fatalf("retry with the same code = %d, want 200 (the refusal consumed it): %s",
			second.Code, second.Body.String())
	}
	if user, _ := repo.GetUserByUsername("fresh-name"); user == nil {
		t.Fatal("successful retry did not create the account")
	}
}

// #17: the duplicate-email path must not burn the code either. The account cannot
// be created with that address, so the observation is that the code is still
// unused afterwards.
func TestRegisterKeepsVerificationCodeAfterDuplicateEmail(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "true")
	t.Setenv("INITIAL_USER_BALANCE", "5")
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

	handler, repo := newSecurityTestHandler(t)
	seedRegistrationUser(t, repo, "email-squatter", "taken@example.invalid")

	const code = "135790"
	if err := repo.SaveVerificationCode("taken@example.invalid", code, "register", 10*time.Minute); err != nil {
		t.Fatalf("save verification code: %v", err)
	}

	refused := registrationRequest(t, handler, "203.0.113.62:1000", "fresh-email-name", "taken@example.invalid", code)
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("duplicate-email registration = %d, want 400: %s", refused.Code, refused.Body.String())
	}
	if !strings.Contains(refused.Body.String(), "该邮箱已被绑定注册") {
		t.Fatalf("message = %s, want the duplicate-email refusal", refused.Body.String())
	}

	// VerifyCode consumes on success, so a true result proves the refusal above
	// never reached it.
	if !repo.VerifyCode("taken@example.invalid", code, "register") {
		t.Fatal("the duplicate-email refusal consumed the still-valid verification code")
	}
}
