package model

import (
	"encoding/json"
	"testing"
)

// A same-protocol rerank upstream may answer with members this struct does not
// model (id, meta) and with documents as bare strings. The canonical round trip
// must not drop or reject them.
func TestRerankResponseRoundTripPreservesUpstreamShape(t *testing.T) {
	upstream := []byte(`{
		"id": "rerank-abc",
		"model": "bge-reranker-v2",
		"results": [
			{"index": 0, "relevance_score": 0.91, "document": "plain string document"},
			{"index": 1, "relevance_score": 0.12, "document": {"text": "object document"}}
		],
		"usage": {"prompt_tokens": 8, "total_tokens": 11},
		"meta": {"billed_units": {"search_units": 1}}
	}`)

	var resp RerankResponse
	if err := json.Unmarshal(upstream, &resp); err != nil {
		t.Fatalf("unmarshal upstream rerank payload: %v", err)
	}
	if resp.Usage.PromptTokens != 8 || resp.Usage.TotalTokens != 11 {
		t.Fatalf("usage not preserved: %+v", resp.Usage)
	}
	if got := resp.Results[0].Document; got == nil || got.Text != "plain string document" {
		t.Fatalf("string document not decoded: %+v", got)
	}
	if got := resp.Results[1].Document; got == nil || got.Text != "object document" {
		t.Fatalf("object document not decoded: %+v", got)
	}

	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal canonical response: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if decoded["id"] != "rerank-abc" {
		t.Errorf("id dropped by canonical round trip: %v", decoded["id"])
	}
	if _, ok := decoded["meta"]; !ok {
		t.Errorf("unknown upstream member dropped by canonical round trip: %v", decoded)
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(8) {
		t.Errorf("usage.prompt_tokens dropped: %v", usage)
	}
}
