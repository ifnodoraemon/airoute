package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestGooglePaymentRequiredIsPreservedBeforeNativeAndChatStreams(t *testing.T) {
	repo := edgeBillingRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		fmt.Fprint(w, `{"error":{"code":402,"status":"RESOURCE_EXHAUSTED","message":"Your prepayment credits are depleted."}}`)
	}))
	defer server.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: server.URL, Models: []string{"m"}}}))
	for _, stream := range []bool{false, true} {
		for _, native := range []bool{false, true} {
			path, body := "/v1/chat/completions", fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
			action := "generateContent"
			if stream {
				action = "streamGenerateContent"
			}
			if native {
				path = "/v1beta/models/m:" + action
				body = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
			}
			c, w := googleResourceContext(t, http.MethodPost, path, body, 101)
			if native {
				c.Params = gin.Params{{Key: "modelAction", Value: "/m:" + action}}
				h.HandleGeminiAction(c)
			} else {
				h.HandleChatCompletions(c)
			}
			if w.Code != 402 || !strings.Contains(w.Body.String(), "prepayment credits are depleted") {
				t.Fatalf("native=%v stream=%v status=%d body=%s", native, stream, w.Code, w.Body.String())
			}
			if native && !strings.Contains(w.Body.String(), `"status":"RESOURCE_EXHAUSTED"`) {
				t.Fatalf("native billing status lost: %s", w.Body.String())
			}
			if !native && !strings.Contains(w.Body.String(), `"code":"upstream_payment_required"`) {
				t.Fatalf("upstream and caller billing were conflated: %s", w.Body.String())
			}
		}
	}
	for _, row := range edgeUsageRows(t, repo, 4) {
		if row.StatusCode != 402 {
			t.Fatalf("upstream billing failure audited as %d", row.StatusCode)
		}
	}
}
