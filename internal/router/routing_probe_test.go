package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestRoutingProbeVerifiesExactMappedModelWithoutFallback(t *testing.T) {
	for _, status := range []int{200, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req model.ChatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Model != "real-model" || req.MaxTokens == nil || *req.MaxTokens != 8 {
					t.Errorf("wrong probe: %+v", req)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"OK"}}]}`)
			}))
			defer server.Close()
			d := NewDispatcher([]model.ChannelConfig{{ID: 1, Name: "gpu", Type: model.ProviderGPUStack, BaseURL: server.URL, Models: []string{"alias"}, ModelMapping: map[string]string{"alias": "real-model"}}})
			for i := 0; i < 3; i++ {
				d.circuitBreaker.RecordFailure("gpu")
			}
			got, err := d.ValidateModelChannel(context.Background(), "alias", 1)
			if err != nil || got.Success != (status == 200) || calls != 1 {
				t.Fatalf("probe=%+v err=%v calls=%d", got, err, calls)
			}
			if d.GetBreakerStatus("gpu") != "OPEN" {
				t.Fatal("manual validation changed production breaker")
			}
			if _, err = d.ValidateModelChannel(context.Background(), "other-model", 1); err != ErrRoutingProbeTarget || calls != 1 {
				t.Fatal("unbound model was probed")
			}
		})
	}
}
