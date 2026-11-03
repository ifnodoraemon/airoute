package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestGeminiLiveBridgesFramesRemapsSetupAndMetersEveryTurn(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "public", GroupName: "default", PromptPrice: 1, CompletionPrice: 2, Currency: "CNY"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "upstream-key" {
			t.Error("Live auth lost")
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, setup, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(string(setup), `"model":"models/actual"`) || !strings.Contains(string(setup), `"vendor":9007199254740993`) {
			t.Errorf("setup changed: %s", setup)
		}
		_ = conn.WriteJSON(map[string]any{"setupComplete": map[string]any{}})
		_, request, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(string(request), "unchanged client content") {
			t.Error("client frame changed")
		}
		for i := 0; i < 2; i++ {
			_ = conn.WriteJSON(map[string]any{"serverContent": map[string]any{"turnComplete": true}, "usageMetadata": map[string]int{"promptTokenCount": 10 + i, "responseTokenCount": 4, "thoughtsTokenCount": 2, "cachedContentTokenCount": 1}})
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
	}))
	defer upstream.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: upstream.URL, APIKey: "upstream-key", Models: []string{"public"}, ModelMapping: map[string]string{"public": "actual"}}}))
	engine := gin.New()
	engine.GET("/ws", func(c *gin.Context) { responsesContextCaller(c, 101, "owner-key"); h.HandleGeminiLive(c) })
	gateway := httptest.NewServer(engine)
	defer gateway.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"setup":{"model":"models/public","generationConfig":{"responseModalities":["AUDIO"]},"vendor":9007199254740993}}`))
	if _, body, err := conn.ReadMessage(); err != nil || !strings.Contains(string(body), "setupComplete") {
		t.Fatalf("setup handshake failed: %s %v", body, err)
	}
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"clientContent":{"turns":[{"parts":[{"text":"unchanged client content"}]}],"turnComplete":true}}`))
	turns := 0
	for {
		_, body, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var frame map[string]any
		_ = json.Unmarshal(body, &frame)
		if frame["usageMetadata"] != nil {
			turns++
		}
	}
	if turns != 2 {
		t.Fatalf("Live turns=%d", turns)
	}
	rows := edgeUsageRows(t, repo, 1)
	if rows[0].PromptTokens != 21 || rows[0].CompletionTokens != 12 || rows[0].CachedTokens != 2 {
		t.Fatalf("Live usage lost: %+v", rows[0])
	}
}

func TestGeminiLiveRejectsDisallowedModelsAndForeignResumeHandles(t *testing.T) {
	repo := edgeBillingRepo(t)
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, Models: []string{"allowed", "private"}}}))
	c, _ := googleResourceContext(t, http.MethodGet, "/ws", "", 202)
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "key-202", UserID: 202, AllowedModels: []string{"allowed"}})
	if _, _, err := h.validateLiveSetup(c, []byte(`{"setup":{"model":"models/private"}}`)); err == nil {
		t.Fatal("Live bypassed model allowlist")
	}
	if err := repo.SaveNativeState(context.Background(), storage.NativeState{Namespace: "gemini-live", ID: "private-handle", Owner: "user:101", Identity: "credential", Model: "allowed", Data: []byte(`{}`), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.validateLiveSetup(c, []byte(`{"setup":{"model":"models/allowed","sessionResumption":{"handle":"private-handle"}}}`)); err == nil {
		t.Fatal("Live resumed another caller's session")
	}
}

func TestGeminiLiveUsageDoesNotTreatVendorErrorAsAcceptedSetup(t *testing.T) {
	stats := &googleLiveUsage{}
	stats.observe([]byte(`{"error":{"code":429,"message":"resource exhausted"}}`))
	if stats.accepted || stats.failure == nil {
		t.Fatal("failed Live setup was treated as success")
	}
	accepted := &googleLiveUsage{}
	accepted.observe([]byte(`{"setupComplete":{}}`))
	if !accepted.accepted || accepted.failure != nil {
		t.Fatal("successful Live setup was not recognized")
	}
}
