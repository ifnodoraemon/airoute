package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestBatchBindingAndAuth(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	handler := NewAdminHandler(repo, nil, nil)

	t.Run("bindBatchIDs valid", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/batch", bytes.NewBufferString(`{"ids": [1, 2, 3]}`))

		ids, ok := handler.bindBatchIDs(c, "empty")
		if !ok || len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Fatalf("expected [1, 2, 3], got %v", ids)
		}
	})

	t.Run("bindBatchIDs empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/batch", bytes.NewBufferString(`{"ids": []}`))

		ids, ok := handler.bindBatchIDs(c, "empty ids")
		if ok || ids != nil {
			t.Fatalf("expected failure on empty ids, got %v", ids)
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("bindBatchStatus active and disabled", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/batch", bytes.NewBufferString(`{"ids": [10], "status": "disabled"}`))

		req, ok := handler.bindBatchStatus(c)
		if !ok || req.Status != "disabled" || len(req.IDs) != 1 || req.IDs[0] != 10 {
			t.Fatalf("expected disabled with ID 10, got %+v", req)
		}

		// Invalid status should fallback to active
		w2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(w2)
		c2.Request, _ = http.NewRequest("POST", "/batch", bytes.NewBufferString(`{"ids": [10], "status": "unknown"}`))

		req2, ok2 := handler.bindBatchStatus(c2)
		if !ok2 || req2.Status != "active" {
			t.Fatalf("expected active fallback for invalid status, got %+v", req2)
		}
	})

	t.Run("filterAuthorizedKeyIDs admin role", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("admin_claims", &AdminClaims{Username: "admin", Role: "admin"})

		ids, ok := handler.filterAuthorizedKeyIDs(c, []int64{101, 102})
		if !ok || len(ids) != 2 {
			t.Fatalf("admin should have full access, got %v", ids)
		}
	})

	t.Run("filterAuthorizedKeyIDs non-admin role", func(t *testing.T) {
		// Create normal user and API key
		user := &storage.UserRecord{
			Username:     "alice",
			PasswordHash: "hash",
			Role:         "user",
			GroupName:    "default",
			Status:       "active",
		}
		if err := repo.CreateUser(user); err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		key1 := &storage.APIKeyRecord{
			Key:       "sk-alice-1",
			UserID:    user.ID,
			GroupName: "default",
			Status:    "active",
		}
		if err := repo.CreateAPIKey(key1); err != nil {
			t.Fatalf("failed to create key: %v", err)
		}

		// Non-admin requesting key1 and non-existent key2
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("admin_claims", &AdminClaims{Username: "alice", Role: "user"})

		ids, ok := handler.filterAuthorizedKeyIDs(c, []int64{key1.ID, 9999})
		if !ok || len(ids) != 1 || ids[0] != key1.ID {
			t.Fatalf("expected filtered IDs [%d], got %v (ok=%v)", key1.ID, ids, ok)
		}

		// Requesting only unauthorized key should be forbidden
		w2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(w2)
		c2.Set("admin_claims", &AdminClaims{Username: "alice", Role: "user"})

		_, ok2 := handler.filterAuthorizedKeyIDs(c2, []int64{9999})
		if ok2 {
			t.Fatalf("expected forbidden for unauthorized key")
		}
		if w2.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w2.Code)
		}
	})
}
