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
