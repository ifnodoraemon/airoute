package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func bcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return string(hash)
}

// resetPasswordRequest invokes the real ResetUserPassword handler as caller.
func resetPasswordRequest(t *testing.T, handler *AdminHandler, caller, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+target+"/password", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "username", Value: target}}
	c.Set("admin_claims", &AdminClaims{Username: caller, Role: "admin"})
	handler.ResetUserPassword(c)
	return w
}

// TestRootAdminPasswordCannotBeResetByAnotherAdmin is the A09 regression.
//
// DeleteUser, UpdateUserStatus and UpdateUserRole all refuse to touch the
// built-in super administrator, but ResetUserPassword did not: any other admin
// could set its password and take the account over.
func TestRootAdminPasswordCannotBeResetByAnotherAdmin(t *testing.T) {
	handler, repo := newSecurityTestHandler(t)
	rootHash := bcryptHash(t, "root-original-password")
	if err := repo.CreateUser(&storage.UserRecord{
		Username: "admin", Email: "root@example.com", PasswordHash: rootHash, Role: "admin", Status: "active",
	}); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := repo.CreateUser(&storage.UserRecord{
		Username: "ops", Email: "ops@example.com", PasswordHash: bcryptHash(t, "ops-password"), Role: "admin", Status: "active",
	}); err != nil {
		t.Fatalf("create ops: %v", err)
	}

	w := resetPasswordRequest(t, handler, "ops", "admin", `{"new_password":"attacker-chosen"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("other admin resetting the root account = %d, want 403 (%s)", w.Code, w.Body.String())
	}
	root, _ := repo.GetUserByUsername("admin")
	if bcrypt.CompareHashAndPassword([]byte(root.PasswordHash), []byte("root-original-password")) != nil {
		t.Fatal("the root password was changed by another administrator")
	}

	// Non-root targets are still administrable.
	if err := repo.CreateUser(&storage.UserRecord{
		Username: "bob", Email: "bob@example.com", PasswordHash: bcryptHash(t, "bob-password"), Role: "user", Status: "active",
	}); err != nil {
		t.Fatalf("create bob: %v", err)
	}
	if w := resetPasswordRequest(t, handler, "ops", "bob", `{"new_password":"bob-new-password"}`); w.Code != http.StatusOK {
		t.Fatalf("admin resetting a normal user = %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

// TestSelfPasswordResetRequiresOldPassword ensures the administrative endpoint
// cannot be used to rotate the caller's own credential without proving knowledge
// of the current one (a hijacked session must not be able to lock the owner out).
func TestSelfPasswordResetRequiresOldPassword(t *testing.T) {
	handler, repo := newSecurityTestHandler(t)
	if err := repo.CreateUser(&storage.UserRecord{
		Username: "admin", Email: "root@example.com", PasswordHash: bcryptHash(t, "root-original-password"),
		Role: "admin", Status: "active",
	}); err != nil {
		t.Fatalf("create root: %v", err)
	}

	if w := resetPasswordRequest(t, handler, "admin", "admin", `{"new_password":"new-password-1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("self reset without old password = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if w := resetPasswordRequest(t, handler, "admin", "admin", `{"new_password":"new-password-1","old_password":"wrong"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("self reset with a wrong old password = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if w := resetPasswordRequest(t, handler, "admin", "admin", `{"new_password":"new-password-1","old_password":"root-original-password"}`); w.Code != http.StatusOK {
		t.Fatalf("self reset with the correct old password = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	root, _ := repo.GetUserByUsername("admin")
	if bcrypt.CompareHashAndPassword([]byte(root.PasswordHash), []byte("new-password-1")) != nil {
		t.Fatal("self reset did not apply the new password")
	}
}
