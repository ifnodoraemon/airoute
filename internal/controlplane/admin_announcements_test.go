package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestSystemAnnouncements(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	handler := NewAdminHandler(repo, nil, nil)

	// 1. Initially no announcement
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/announcement", nil)

		handler.GetAnnouncement(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Code int                 `json:"code"`
			Data *SystemAnnouncement `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Data != nil {
			t.Fatalf("expected nil announcement, got %+v", resp.Data)
		}
	}

	// 2. Save active announcement
	{
		body := map[string]any{
			"title":   "升级维护通知",
			"content": "今晚 24:00 进行网络升级",
			"level":   "warning",
			"enabled": true,
		}
		bytesData, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/v1/admin/announcement", bytes.NewReader(bytesData))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.SaveAnnouncement(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on save, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 3. Get public active announcement
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/announcement", nil)

		handler.GetAnnouncement(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Code int                 `json:"code"`
			Data *SystemAnnouncement `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Data == nil || resp.Data.Title != "升级维护通知" || resp.Data.Level != "warning" {
			t.Fatalf("unexpected announcement data: %+v", resp.Data)
		}
	}

	// 4. Disable announcement
	{
		body := map[string]any{
			"title":   "升级维护通知",
			"content": "今晚 24:00 进行网络升级",
			"level":   "warning",
			"enabled": false,
		}
		bytesData, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/v1/admin/announcement", bytes.NewReader(bytesData))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.SaveAnnouncement(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on save disable, got %d", w.Code)
		}
	}

	// 5. Public should return nil when disabled
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/announcement", nil)

		handler.GetAnnouncement(c)

		var resp struct {
			Code int                 `json:"code"`
			Data *SystemAnnouncement `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Data != nil {
			t.Fatalf("expected nil when disabled, got %+v", resp.Data)
		}
	}

	// 6. Admin should still see the announcement config even when disabled
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/admin/announcement/config", nil)

		handler.GetAdminAnnouncement(c)

		var resp struct {
			Code int                `json:"code"`
			Data SystemAnnouncement `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Data.Title != "升级维护通知" || resp.Data.Enabled != false {
			t.Fatalf("expected disabled config for admin, got %+v", resp.Data)
		}
	}
}
