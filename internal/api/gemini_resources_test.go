package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func googleResourceContext(t *testing.T, method, path, body string, owner int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	c, w := newTestContext(method, path)
	c.Request.Body = io.NopCloser(strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	responsesContextCaller(c, owner, fmt.Sprintf("key-%d", owner))
	return c, w
}

func TestGoogleNativeResourcesAreOwnedAndStayOnTheirOriginalCredential(t *testing.T) {
	_ = edgeBillingRepo(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("x-goog-api-key") != "original-key" {
			t.Error("resource changed credentials")
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"model":"models/actual"`) || !strings.Contains(string(body), `"opaque":9007199254740993`) {
				t.Errorf("cache request changed: %s", body)
			}
		}
		fmt.Fprint(w, `{"name":"cachedContents/cache-a","model":"models/actual","opaque":9007199254740993}`)
	}))
	defer upstream.Close()
	channel := model.ChannelConfig{Name: "google", Type: model.ProviderGemini, BaseURL: upstream.URL, APIKey: "original-key", Models: []string{"public"}, ModelMapping: map[string]string{"public": "actual"}}
	d := router.NewDispatcher([]model.ChannelConfig{channel})
	h := NewHandler(d)
	c, w := googleResourceContext(t, http.MethodPost, "/v1beta/cachedContents", `{"model":"models/public","contents":[{"parts":[{"text":"private"}]}],"opaque":9007199254740993}`, 101)
	h.HandleGeminiResource(c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cache-a") {
		t.Fatalf("cache create failed: %d %s", w.Code, w.Body.String())
	}
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/cachedContents/cache-a", "", 202)
	h.HandleGeminiResource(c)
	if w.Code != 404 || calls.Load() != 1 {
		t.Fatal("foreign resource was forwarded")
	}
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/cachedContents", "", 202)
	h.HandleGeminiResource(c)
	if w.Code != 200 || strings.Contains(w.Body.String(), "cache-a") || calls.Load() != 1 {
		t.Fatal("listing exposed an upstream account")
	}
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/cachedContents/cache-a", "", 101)
	h.HandleGeminiResource(c)
	if w.Code != 200 || calls.Load() != 2 {
		t.Fatalf("owner cannot retrieve resource: %d %s", w.Code, w.Body.String())
	}
	channel.APIKey = "rotated-key"
	d.UpdateChannels([]model.ChannelConfig{channel})
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/cachedContents/cache-a", "", 101)
	h.HandleGeminiResource(c)
	if w.Code == 200 || calls.Load() != 2 {
		t.Fatal("resource moved to another upstream credential")
	}
}

func TestGoogleResumableUploadURLIsScopedAndProxyDoesNotExposeUploadTokens(t *testing.T) {
	_ = edgeBillingRepo(t)
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Upload-Command") == "upload, finalize" {
			if r.URL.Query().Get("upload_id") != "private-upload-token" {
				t.Error("upload continuation lost")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "image-bytes" {
				t.Errorf("upload body changed: %q", body)
			}
			fmt.Fprint(w, `{"file":{"name":"files/owned-file","uri":"https://generativelanguage.googleapis.com/v1beta/files/owned-file","mimeType":"image/png"}}`)
			return
		}
		w.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload/v1beta/files?upload_id=private-upload-token")
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: upstream.URL, APIKey: "upstream-key", Models: []string{"m"}}}))
	c, w := googleResourceContext(t, http.MethodPost, "/upload/v1beta/files", `{"file":{"displayName":"image"}}`, 101)
	c.Request.Header.Set("X-Goog-Upload-Protocol", "resumable")
	h.HandleGeminiUpload(c)
	location := w.Header().Get("X-Goog-Upload-URL")
	if w.Code != 200 || location == "" || strings.Contains(location, "private-upload-token") {
		t.Fatalf("unsafe upload URL: %d %s", w.Code, location)
	}
	u, _ := url.Parse(location)
	id := strings.TrimPrefix(u.Path, "/upload/v1beta/sessions/")
	c, w = googleResourceContext(t, http.MethodPost, u.Path, "image-bytes", 202)
	c.Params = gin.Params{{Key: "upload", Value: id}}
	h.HandleGeminiUpload(c)
	if w.Code != 404 {
		t.Fatal("foreign upload continuation was accepted")
	}
	c, w = googleResourceContext(t, http.MethodPost, u.Path, "image-bytes", 101)
	c.Params = gin.Params{{Key: "upload", Value: id}}
	c.Request.Header.Set("Content-Type", "image/png")
	c.Request.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	h.HandleGeminiUpload(c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "owned-file") {
		t.Fatalf("upload finalize failed: %d %s", w.Code, w.Body.String())
	}
}

func TestGoogleBatchMeteringReconcilesWithoutClientPollingAndChargesOnce(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "m", GroupName: "default", PromptPrice: 1, CompletionPrice: 2, Currency: "CNY"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `{"name":"batches/job-a","done":false}`)
			return
		}
		fmt.Fprint(w, `{"name":"batches/job-a","done":true,"response":{"inlinedResponses":[{"response":{"candidates":[],"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":8,"thoughtsTokenCount":3,"totalTokenCount":28}}}]}}`)
	}))
	defer server.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: server.URL, Models: []string{"m"}}}))
	c, w := googleResourceContext(t, http.MethodPost, "/v1beta/models/m:batchGenerateContent", `{"batch":{"displayName":"a","inputConfig":{"requests":{"requests":[{"request":{"contents":[{"parts":[{"text":"hi"}]}]}}]}}}}`, 101)
	h.handleGeminiBatchAction(c, "m", "batchGenerateContent")
	if w.Code != 200 {
		t.Fatalf("batch submit: %d %s", w.Code, w.Body.String())
	}
	jobs, err := repo.PendingNativeJobs(context.Background())
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pending job lost: %v %+v", err, jobs)
	}
	reconcileNativeJobs(context.Background(), h)
	reconcileNativeJobs(context.Background(), h)
	rows := edgeUsageRows(t, repo, 1)
	if rows[0].PromptTokens != 17 || rows[0].CompletionTokens != 11 {
		t.Fatalf("batch usage lost: %+v", rows[0])
	}
	jobs, err = repo.PendingNativeJobs(context.Background())
	if err != nil || len(jobs) != 0 {
		t.Fatal("completed job remained pending")
	}
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/batches/job-a", "", 101)
	h.HandleGeminiResource(c)
	if w.Code != 200 {
		t.Fatalf("poll completed batch: %d %s", w.Code, w.Body.String())
	}
	if got := edgeUsageRows(t, repo, 1); len(got) != 1 {
		t.Fatalf("batch billed again: %+v", got)
	}
}

func TestGeminiCatalogUsesMappedModelsAndUpstreamCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/vectors-real") {
			fmt.Fprint(w, `{"name":"models/vectors-real","inputTokenLimit":2048,"supportedGenerationMethods":["embedContent","batchEmbedContents","futureMethod"]}`)
		} else {
			fmt.Fprint(w, `{"name":"models/chat-real","supportedGenerationMethods":["generateContent","streamGenerateContent","countTokens"]}`)
		}
	}))
	defer server.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: server.URL, Models: []string{"chat", "vectors"}, ModelMapping: map[string]string{"chat": "chat-real", "vectors": "vectors-real"}, Protocols: []string{"openai_chat", "embeddings"}}}))
	c, w := googleResourceContext(t, http.MethodGet, "/v1beta/models", "", 101)
	h.HandleGeminiModels(c)
	var response struct {
		Models []struct {
			Name    string   `json:"name"`
			Methods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	if len(response.Models) != 2 {
		t.Fatalf("wrong catalog: %s", w.Body.String())
	}
	for _, item := range response.Models {
		methods := strings.Join(item.Methods, ",")
		if item.Name == "models/vectors" && (strings.Contains(methods, "generateContent") || strings.Contains(methods, "futureMethod") || !strings.Contains(methods, "embedContent")) {
			t.Fatalf("wrong vector capabilities: %+v", item)
		}
	}
}
