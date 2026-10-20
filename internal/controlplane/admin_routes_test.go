package controlplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Pins the HTTP contract of the model-route save endpoint: the status code and
// the stage-specific message the console shows an operator. "Nothing was saved"
// and "saved, but the data plane did not pick it up" are different situations,
// so each stage keeps its own wording.

func newRouteTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func invokeRoute(t *testing.T, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/admin/models/routes", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler(c)
	return w
}

func routeError(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON (%d): %s", w.Code, w.Body.String())
	}
	msg, _ := out["error"].(string)
	return msg
}

func TestUpdateModelRouteHTTPContract(t *testing.T) {
	t.Run("saves and reports success", func(t *testing.T) {
		handler, repo := newRouteTestHandler(t)
		if err := repo.CreateChannel(&storage.ChannelRecord{
			Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
			Models: []string{"gpt-x"}, Priority: 1, Weight: 5, Status: "active",
		}); err != nil {
			t.Fatalf("seed channel: %v", err)
		}

		w := invokeRoute(t, handler.UpdateModelRoute, `{"model":"gpt-x","prompt_price":1.5}`)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}
		var out map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("response is not JSON: %v", err)
		}
		if code, _ := out["code"].(float64); code != 0 {
			t.Errorf("envelope code = %v, want 0", out["code"])
		}
		if msg, _ := out["message"].(string); !strings.Contains(msg, "已保存") {
			t.Errorf("message = %q, want a saved confirmation", msg)
		}

		// The write really landed.
		price, err := repo.GetModelPriceExact("gpt-x", "default")
		if err != nil {
			t.Fatalf("price was not persisted: %v", err)
		}
		if price.PromptPrice != 1.5 {
			t.Errorf("prompt price = %v, want 1.5", price.PromptPrice)
		}
	})

	t.Run("rejects an empty model with 400", func(t *testing.T) {
		handler, _ := newRouteTestHandler(t)

		w := invokeRoute(t, handler.UpdateModelRoute, `{"model":"   "}`)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
		}
		if msg := routeError(t, w); msg != "模型标识不能为空" {
			t.Errorf("error = %q, want the documented message", msg)
		}
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		handler, _ := newRouteTestHandler(t)

		w := invokeRoute(t, handler.UpdateModelRoute, `{not json`)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects overlapping off-peak slots with 400", func(t *testing.T) {
		handler, _ := newRouteTestHandler(t)

		w := invokeRoute(t, handler.UpdateModelRoute,
			`{"model":"gpt-x","off_peak_slots":"00:00-10:00,05:00-12:00"}`)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
		}
	})
}

func TestUpdateModelRouteStageMessages(t *testing.T) {
	// White-box: the handler's route service is replaced with one backed by a
	// failing store so each stage's message can be asserted without provoking a
	// real database failure.
	cases := []struct {
		name        string
		store       *fakeRouteStore
		sync        *fakeRouteSync
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "collect failure reports a channel problem",
			store:       &fakeRouteStore{prices: map[string]*storage.ModelPriceRecord{}, listErr: errors.New("boom")},
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "更新渠道配置失败: boom",
		},
		{
			name:        "persist failure reports that nothing changed",
			store:       &fakeRouteStore{prices: map[string]*storage.ModelPriceRecord{}, applyErr: errors.New("boom")},
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "保存模型路由失败（已回滚，未改动任何配置）: boom",
		},
		{
			name:        "apply failure reports the data plane did not reload",
			store:       &fakeRouteStore{prices: map[string]*storage.ModelPriceRecord{}},
			sync:        &fakeRouteSync{err: errors.New("boom")},
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "数据面热重载失败: boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := newRouteTestHandler(t)
			handler.routes = NewRouteService(tc.store, tc.sync, nil)

			w := invokeRoute(t, handler.UpdateModelRoute, `{"model":"gpt-x"}`)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if msg := routeError(t, w); msg != tc.wantMessage {
				t.Errorf("error = %q, want %q", msg, tc.wantMessage)
			}
		})
	}
}
