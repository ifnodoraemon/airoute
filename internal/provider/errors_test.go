package provider

import "testing"

// A status-only upstream failure used to render "upstream ch returned status 404: "
// with a dangling colon, which read like a message that had been truncated.
func TestUpstreamErrorWithoutBodyDoesNotDangle(t *testing.T) {
	if got := NewUpstreamError("罗总", 404, nil).Error(); got != "upstream 罗总 returned status 404" {
		t.Fatalf("empty body left a dangling suffix, got %q", got)
	}
	if got := NewUpstreamError("", 503, nil).Error(); got != "upstream returned status 503" {
		t.Fatalf("unnamed channel format changed, got %q", got)
	}
	// A body is still appended after the status, unchanged.
	if got := NewUpstreamError("ch", 400, []byte(`{"error":"bad"}`)).Error(); got != `upstream ch returned status 400: {"error":"bad"}` {
		t.Fatalf("body formatting changed, got %q", got)
	}
}

// B-03: every dispatch path must classify a 404 as "this node does not host the
// model, try the next channel", never as a deterministic client error that ends
// the fan-out. Only 400/413/422 are fatal payload errors.
func TestPayloadClassificationKeeps404Retryable(t *testing.T) {
	if !IsModelNotFound(NewUpstreamError("node-a", 404, nil)) {
		t.Fatal("404 must be recognised as a node-specific model miss")
	}
	if IsFatalPayloadError(NewUpstreamError("node-a", 404, nil)) {
		t.Fatal("404 must not be fatal: another node may host the model")
	}
	for _, status := range []int{400, 413, 422} {
		if !IsFatalPayloadError(NewUpstreamError("ch", status, nil)) {
			t.Fatalf("%d must be a fatal payload error", status)
		}
	}
	for _, status := range []int{401, 403, 408, 429, 500} {
		if IsFatalPayloadError(NewUpstreamError("ch", status, nil)) {
			t.Fatalf("%d must not be a fatal payload error", status)
		}
	}
	if IsModelNotFound(nil) || IsFatalPayloadError(nil) {
		t.Fatal("nil error must not be classified as an upstream status")
	}
}
