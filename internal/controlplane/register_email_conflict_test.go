package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// A duplicate email surfaced by the repository (the concurrent-registration
// path guarded by the unique index) must be a deterministic client error, not a
// generic 500.
func TestRespondRegistrationFailureMapsDuplicateEmailToConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"duplicate email", storage.ErrEmailAlreadyRegistered, http.StatusConflict},
		{"wrapped duplicate email", errors.Join(errors.New("insert user"), storage.ErrEmailAlreadyRegistered), http.StatusConflict},
		{"other failure", errors.New("database is on fire"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondRegistrationFailure(c, tc.err)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.wantStatus, w.Body.String())
			}
			var payload struct {
				Code  int    `json:"code"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload.Code != tc.wantStatus {
				t.Fatalf("payload code = %d, want %d", payload.Code, tc.wantStatus)
			}
			if payload.Error == "" {
				t.Fatal("expected a human-readable error message")
			}
		})
	}
}

// End-to-end: registering twice with the same address must create exactly one
// account, and the second attempt must be refused with a conflict.
func TestRegisterRejectsSecondAccountForSameEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo := regressionHandler(t)

	first := registerBody("first-user", "shared@example.com")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(first))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusOK {
		t.Fatalf("first registration failed: status=%d body=%s", w.Code, w.Body.String())
	}

	second := registerBody("second-user", "SHARED@example.com")
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(second))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Register(c2)
	if w2.Code == http.StatusOK {
		t.Fatalf("second registration for the same email succeeded: %s", w2.Body.String())
	}
	if w2.Code != http.StatusBadRequest && w2.Code != http.StatusConflict {
		t.Fatalf("unexpected status for duplicate email: %d body=%s", w2.Code, w2.Body.String())
	}

	users, err := repo.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	count := 0
	for _, u := range users {
		if strings.EqualFold(strings.TrimSpace(u.Email), "shared@example.com") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one account for the address, got %d", count)
	}
}

func registerBody(username, email string) string {
	return `{"username":"` + username + `","email":"` + email + `","password":"secret123"}`
}
