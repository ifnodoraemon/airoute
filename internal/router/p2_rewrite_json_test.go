package router

import (
	"strings"
	"testing"
)

// R-14: the model rewrite decoded into map[string]any and re-marshalled, turning
// every integer into float64. 64-bit seeds/timestamps were silently altered.
func TestP2RewriteJSONModelPreservesIntegerPrecision(t *testing.T) {
	body := []byte(`{"model":"alias","seed":9007199254740993,"nested":{"id":18446744073709551615},"ratio":0.25,"name":"job"}`)
	out := RewriteJSONModel(body, "upstream-model")
	got := string(out)
	for _, want := range []string{
		`"model":"upstream-model"`,
		`"seed":9007199254740993`,
		`"id":18446744073709551615`,
		`"ratio":0.25`,
		`"name":"job"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten payload %s lost %s", got, want)
		}
	}
}

// The rewrite must stay a no-op for models that do not need mapping.
func TestP2RewriteJSONModelNoOpKeepsOriginalBytes(t *testing.T) {
	body := []byte(`{"model":"same","seed":9007199254740993}`)
	if got := string(RewriteJSONModel(body, "same")); got != string(body) {
		t.Fatalf("no-op rewrite changed the payload: %s", got)
	}
}
