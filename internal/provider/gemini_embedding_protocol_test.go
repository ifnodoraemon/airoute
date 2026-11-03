package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestGeminiEmbeddingDimensionsReachSingleAndBatchEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Dimensions int `json:"outputDimensionality"`
			Requests   []struct {
				Dimensions int `json:"outputDimensionality"`
			} `json:"requests"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, ":batchEmbedContents") {
			if len(body.Requests) != 2 || body.Requests[0].Dimensions != 2 || body.Requests[1].Dimensions != 2 {
				t.Errorf("batch dimensions lost: %+v", body)
			}
			fmt.Fprint(w, `{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}]}`)
		} else {
			if body.Dimensions != 2 {
				t.Errorf("dimensions lost: %+v", body)
			}
			fmt.Fprint(w, `{"embedding":{"values":[0.1,0.2]}}`)
		}
	}))
	defer server.Close()
	dimension := 2
	for _, input := range []any{"single", []string{"first", "second"}} {
		response, err := NewGeminiProvider(server.Client()).Embed(context.Background(), &model.EmbeddingRequest{Model: "m", Input: input, Dimensions: &dimension}, &model.ChannelConfig{BaseURL: server.URL})
		if err != nil {
			t.Fatal(err)
		}
		if len(response.Data[0].Embedding) != 2 {
			t.Fatalf("wrong dimensions: %+v", response)
		}
	}
}

func TestGeminiProtoJSONAliasesPreserveUserKeysAndNumbers(t *testing.T) {
	body := CanonicalGeminiJSON([]byte(`{"generation_config":{"max_output_tokens":50,"response_json_schema":{"properties":{"user_name":{"type":"string"}}}},"contents":[{"parts":[{"function_call":{"name":"lookup","args":{"user_name":"a","exact_id":9007199254740993}}}]}]}`))
	for _, want := range []string{`"generationConfig"`, `"maxOutputTokens":50`, `"functionCall"`, `"user_name"`, `"exact_id":9007199254740993`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("ProtoJSON normalization lost %s: %s", want, body)
		}
	}
	if strings.Contains(string(body), `"userName"`) {
		t.Fatalf("user schema or function argument keys changed: %s", body)
	}
}
