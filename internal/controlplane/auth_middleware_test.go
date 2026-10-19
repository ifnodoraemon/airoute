package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestAuthRejectsDeletedAndDemotedUsers(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	repo := storage.NewRepository(db)
	user := &storage.UserRecord{Username: "operator", Role: "admin", Status: "active", PasswordHash: "test"}
	if err := repo.CreateUser(user); err != nil { t.Fatal(err) }
	handler := NewAdminHandler(repo, nil, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.AdminAuthMiddleware())
	r.GET("/private", func(c *gin.Context) { c.Status(http.StatusOK) })
	token, err := GenerateAdminToken("operator", "admin", time.Hour)
	if err != nil { t.Fatal(err) }
	check := func(want int) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Authorization", "Bearer " + token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want { t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String()) }
	}
	check(http.StatusOK)
	if err := repo.UpdateUserRole("operator", "user"); err != nil { t.Fatal(err) }
	check(http.StatusUnauthorized)
	if err := repo.DeleteUser("operator"); err != nil { t.Fatal(err) }
	check(http.StatusUnauthorized)
}
