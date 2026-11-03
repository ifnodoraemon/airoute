package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestGoogleBatchResultFileIsOwnedAndMetered(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "m", GroupName: "default", PromptPrice: 1, CompletionPrice: 2, Currency: "CNY"})
	var downloads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":download") {
			downloads.Add(1)
			if r.Header.Get("x-goog-api-key") != "original" {
				t.Error("result file used another credential")
			}
			fmt.Fprintln(w, `{"key":"a","response":{"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":5,"thoughtsTokenCount":2,"totalTokenCount":19}}}`)
			fmt.Fprintln(w, `{"key":"b","response":{"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":3,"totalTokenCount":12}}}`)
			return
		}
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `{"name":"operations/job-file","done":false}`)
			return
		}
		fmt.Fprint(w, `{"name":"operations/job-file","done":true,"metadata":{"output":{"responsesFile":"files/result-file"}}}`)
	}))
	defer upstream.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: upstream.URL, APIKey: "original", Models: []string{"m"}}}))
	c, w := googleResourceContext(t, http.MethodPost, "/v1beta/models/m:batchGenerateContent", `{"batch":{"inputConfig":{"requests":{"requests":[]}}}}`, 101)
	h.handleGeminiBatchAction(c, "m", "batchGenerateContent")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	reconcileNativeJobs(context.Background(), h)
	rows := edgeUsageRows(t, repo, 1)
	if rows[0].PromptTokens != 21 || rows[0].CompletionTokens != 10 || downloads.Load() != 1 {
		t.Fatalf("file usage missing: %+v downloads=%d", rows[0], downloads.Load())
	}
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/files/result-file:download?alt=media", "", 202)
	h.HandleGeminiResource(c)
	if w.Code != 404 || downloads.Load() != 1 {
		t.Fatal("foreign caller downloaded batch output")
	}
	c, w = googleResourceContext(t, http.MethodGet, "/v1beta/operations/job-file", "", 101)
	h.HandleGeminiResource(c)
	if w.Code != 200 || downloads.Load() != 1 {
		t.Fatalf("operation cannot be polled or was charged again: %d %s", w.Code, w.Body.String())
	}
}
