package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestNativeGeminiRetainsSDKFeaturesAcrossGenerateCountAndEmbed(t *testing.T) {
	_ = edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "m", GroupName: "default", PromptPrice: 1, CompletionPrice: 2, Currency: "CNY"})
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("x-goog-api-key") != "upstream-key" || !strings.HasPrefix(r.Header.Get("x-goog-api-client"), "airoute/") {
			t.Error("missing Google authentication/client headers")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "9007199254740993") {
			t.Errorf("unknown integer field lost: %s", body)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, ":countTokens"):
			if !strings.Contains(string(body), `"model":"models/actual"`) {
				t.Errorf("wrapped count model alias lost: %s", body)
			}
			fmt.Fprint(w, `{"totalTokens":321,"cachedContentTokenCount":123}`)
		case strings.HasSuffix(r.URL.Path, ":embedContent"):
			if !strings.Contains(string(body), `"inline_data"`) || !strings.Contains(string(body), `"task_type":"RETRIEVAL_DOCUMENT"`) {
				t.Errorf("multimodal embedding changed: %s", body)
			}
			fmt.Fprint(w, `{"embedding":{"values":[0.1,0.2]},"usageMetadata":{"promptTokenCount":42},"vendor":9007199254740993}`)
		case strings.HasSuffix(r.URL.Path, ":streamGenerateContent"):
			fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"\",\"thoughtSignature\":\"empty-part-signature==\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":2}}\n\n")
		default:
			if !strings.Contains(string(body), `"google_search"`) || !strings.Contains(string(body), `"cached_content"`) {
				t.Errorf("native tools/context lost: %s", body)
			}
			fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"answer","thoughtSignature":"sig=="}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2},"vendor":9007199254740993}`)
		}
	}))
	defer upstream.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: upstream.URL, APIKey: "upstream-key", Models: []string{"m"}, ModelMapping: map[string]string{"m": "actual"}}}))
	cases := []struct{ action, body, want string }{
		{"generateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"tools":[{"google_search":{}}],"cached_content":"cachedContents/x","vendor":9007199254740993}`, `"thoughtSignature":"sig=="`},
		{"streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"vendor":9007199254740993}`, `"thoughtSignature":"empty-part-signature=="`},
		{"countTokens", `{"generateContentRequest":{"model":"models/m","contents":[{"parts":[{"text":"hi"}]}]},"vendor":9007199254740993}`, `"totalTokens":321`},
		{"embedContent", `{"content":{"parts":[{"inline_data":{"mime_type":"image/png","data":"aGVsbG8="}}]},"task_type":"RETRIEVAL_DOCUMENT","output_dimensionality":2,"vendor":9007199254740993}`, `"vendor":9007199254740993`},
	}
	for _, test := range cases {
		t.Run(test.action, func(t *testing.T) {
			c, w := newTestContext(http.MethodPost, "/v1beta/models/m:"+test.action)
			c.Params = gin.Params{{Key: "modelAction", Value: "/m:" + test.action}}
			responsesContextCaller(c, 101, "owner-key")
			if test.action == "generateContent" {
				channels := h.dispatcher.ListChannelsForModel("m", "")
				if err := storage.GetGlobalRepository().SaveNativeState(context.Background(), storage.NativeState{Namespace: "gemini-cache", ID: "cachedContents/x", Owner: "user:101", Identity: model.UpstreamIdentity(channels[0]), Model: "m", Data: []byte(`{}`), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
					t.Fatal(err)
				}
			}
			c.Request.Body = io.NopCloser(strings.NewReader(test.body))
			h.HandleGeminiAction(c)
			if w.Code != 200 || !strings.Contains(w.Body.String(), test.want) {
				t.Fatalf("native %s failed: %d %s", test.action, w.Code, w.Body.String())
			}
			if test.action == "countTokens" && w.Header().Get("X-Airoute-Token-Estimate") != "" {
				t.Fatal("exact upstream count marked estimated")
			}
		})
	}
	if calls.Load() != 4 {
		t.Fatalf("expected exactly one upstream call per action, got %d", calls.Load())
	}
}

func TestGeminiNativeOnlyFeaturesRejectLossyFallback(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer upstream.Close()
	_ = edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "m", GroupName: "default", PromptPrice: 1, CompletionPrice: 2, Currency: "CNY"})
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "chat-only", Type: model.ProviderOpenAI, BaseURL: upstream.URL, Models: []string{"m"}, Protocols: []string{"openai_chat"}}}))
	c, w := newTestContext(http.MethodPost, "/v1beta/models/m:generateContent")
	c.Params = gin.Params{{Key: "modelAction", Value: "/m:generateContent"}}
	c.Request.Body = io.NopCloser(strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"search"}]}],"tools":[{"googleSearch":{}}]}`))
	h.HandleGeminiAction(c)
	var body struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 400 || body.Error.Status != "INVALID_ARGUMENT" || calls.Load() != 0 {
		t.Fatalf("lossy fallback was allowed: status=%d body=%s calls=%d", w.Code, w.Body.String(), calls.Load())
	}
}
