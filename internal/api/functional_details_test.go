package api

import (
	"encoding/json"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

type auditReviewTransport func(*http.Request) (*http.Response, error)

func (f auditReviewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func auditReviewResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func auditReviewMultimodal(t *testing.T, channels []model.ChannelConfig, transport auditReviewTransport) *MultimodalHandler {
	t.Helper()
	previousClient, previousEngine := provider.SharedDefaultHTTPClient, billing.GlobalEngine
	provider.SharedDefaultHTTPClient = &http.Client{Transport: transport}
	billing.GlobalEngine = nil
	t.Cleanup(func() { provider.SharedDefaultHTTPClient = previousClient; billing.GlobalEngine = previousEngine })
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewMultimodalHandler(router.NewDispatcher(channels), storage.NewRepository(db))
}

func TestAuditReviewMultimodalPreservesAPIBasePath(t *testing.T) {
	var calledPath string
	h := auditReviewMultimodal(t, []model.ChannelConfig{{Name: "multi", BaseURL: "https://audit.invalid/v1", Models: []string{"image-model"}, Protocols: []string{"images"}, Status: "active"}}, func(r *http.Request) (*http.Response, error) {
		calledPath = r.URL.Path
		return auditReviewResponse(200, `{"data":[{"url":"https://asset.invalid/image.png"}]}`), nil
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{"model":"image-model","prompt":"hello"}`))
	h.HandleImageGenerations(c)
	if calledPath != "/v1/images/generations" {
		t.Fatalf("base URL ending /v1 produced upstream path %s", calledPath)
	}
}

func TestAuditReviewVideoPollReturnsToCreatingProvider(t *testing.T) {
	var creator, poller string
	h := auditReviewMultimodal(t, []model.ChannelConfig{
		{Name: "video-a", BaseURL: "https://video-a.invalid", Models: []string{"video-model"}, Protocols: []string{"videos"}, Status: "active", Weight: 10, Priority: 1},
		{Name: "video-b", BaseURL: "https://video-b.invalid", Models: []string{"video-model"}, Protocols: []string{"videos"}, Status: "active", Weight: 10, Priority: 1},
	}, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			creator = r.URL.Host
			return auditReviewResponse(200, `{"id":"audit-task","status":"processing"}`), nil
		}
		poller = r.URL.Host
		if poller != creator {
			return auditReviewResponse(404, `{"error":{"message":"task not found on this provider"}}`), nil
		}
		return auditReviewResponse(200, `{"id":"audit-task","status":"succeeded"}`), nil
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/videos/generations", strings.NewReader(`{"model":"video-model","prompt":"hello"}`))
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-audit-owner", UserID: 1})
	h.HandleVideoGenerations(c)
	if w.Code != 200 {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	// A new handler sharing storage simulates polling on another replica.
	h = NewMultimodalHandler(h.dispatcher, storage.NewRepository(h.repo.GetDB()))
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/videos/tasks/"+payload.ID+"?model=video-model", nil)
	c.Params = gin.Params{{Key: "id", Value: payload.ID}}
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-audit-owner", UserID: 1})
	h.HandleVideoTask(c)
	if w.Code != 200 || poller != creator {
		t.Fatalf("created on %s, polled on %s, status=%d", creator, poller, w.Code)
	}
}

func TestVideoTaskOwnershipAndPinnedFailure(t *testing.T) {
	calls := 0
	upstreamFails := false
	channels := []model.ChannelConfig{
		{ID: 1, Name: "video-a", BaseURL: "https://a.invalid/v1", Models: []string{"sora"}, Protocols: []string{"videos"}, Status: "active", Weight: 10},
		{ID: 2, Name: "video-b", BaseURL: "https://b.invalid/v1", Models: []string{"sora"}, Protocols: []string{"videos"}, Status: "active", Weight: 10},
	}
	h := auditReviewMultimodal(t, channels, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" {
			return auditReviewResponse(200, `{"id":"provider-task","status":"processing"}`), nil
		}
		if upstreamFails {
			return auditReviewResponse(503, `{"error":"unavailable"}`), nil
		}
		return auditReviewResponse(200, `{"id":"provider-task","status":"succeeded"}`), nil
	})
	invoke := func(method, path, id string, user int64, models []string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(`{"model":"sora","prompt":"hello"}`))
		c.Params = gin.Params{{Key: "id", Value: id}}
		if user > 0 {
			c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-test", UserID: user, AllowedModels: models})
		}
		if method == "POST" {
			h.HandleVideoGenerations(c)
		} else {
			h.HandleVideoTask(c)
		}
		return w
	}
	created := invoke("POST", "/v1/videos/generations", "", 1, nil)
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(payload.ID, "vid_") {
		t.Fatalf("provider ID exposed: %s", payload.ID)
	}
	// The upstream may reuse a task ID. Each gateway task remains distinct.
	second := invoke("POST", "/v1/videos/generations", "", 1, nil)
	var duplicate struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(second.Body.Bytes(), &duplicate)
	if duplicate.ID == payload.ID || duplicate.ID == "" {
		t.Fatal("upstream ID collision lost a task")
	}
	for _, tt := range []struct {
		name, query, id string
		owner           int64
		models          []string
		want            int
	}{
		{name: "other owner", id: payload.ID, owner: 2, want: 404},
		{name: "unknown", id: "vid_missing", owner: 1, want: 404},
		{name: "anonymous", id: payload.ID, want: 401},
		{name: "model mismatch", id: payload.ID, query: "?model=other", owner: 1, want: 400},
		{name: "model revoked", id: payload.ID, owner: 1, models: []string{"other"}, want: 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := calls
			w := invoke("GET", "/v1/videos/tasks/"+tt.id+tt.query, tt.id, tt.owner, tt.models)
			if w.Code != tt.want || calls != before {
				t.Fatalf("status=%d calls=%d->%d: %s", w.Code, before, calls, w.Body.String())
			}
		})
	}
	before := calls
	w := invoke("GET", "/v1/videos/tasks/"+payload.ID, payload.ID, 1, nil)
	if w.Code != 200 || calls != before+1 {
		t.Fatalf("stored model lookup failed: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), payload.ID) {
		t.Fatal("poll response lost gateway task ID")
	}
	upstreamFails = true
	before = calls
	w = invoke("GET", "/v1/videos/tasks/"+payload.ID, payload.ID, 1, nil)
	if w.Code != 502 || calls != before+1 {
		t.Fatalf("task query failed over: status=%d calls=%d->%d", w.Code, before, calls)
	}
}

func TestLegacyForeignPriceIsRejectedBeforeUpstream(t *testing.T) {
	called := false
	h := auditReviewMultimodal(t, []model.ChannelConfig{{Name: "image", BaseURL: "https://a.invalid/v1", Models: []string{"foreign"}, Protocols: []string{"images"}}}, func(r *http.Request) (*http.Response, error) {
		called = true
		return auditReviewResponse(200, `{}`), nil
	})
	if err := h.repo.SaveModelPrice(&storage.ModelPriceRecord{Model: "foreign", Currency: "CNY", FixedPrice: 1}); err != nil {
		t.Fatal(err)
	}
	// Simulate an already-persisted foreign rate from before this fix.
	if _, err := h.repo.GetDB().Exec(`UPDATE model_prices SET currency='USD' WHERE model='foreign'`); err != nil {
		t.Fatal(err)
	}
	billing.GlobalEngine = billing.NewEngine(h.repo)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{"model":"foreign","prompt":"hello"}`))
	h.HandleImageGenerations(c)
	if w.Code != 422 || called {
		t.Fatalf("foreign rate reached upstream: status=%d called=%v", w.Code, called)
	}
}
