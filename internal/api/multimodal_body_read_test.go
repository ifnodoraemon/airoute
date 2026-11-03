package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// failingReadCloser models an upstream body that fails mid-read (reset
// connection, truncated chunked response).
type failingReadCloser struct{ err error }

func (f failingReadCloser) Read([]byte) (int, error) { return 0, f.err }
func (f failingReadCloser) Close() error             { return nil }

// The multimodal proxy used to record success + billing and flush HTTP 200
// before io.Copy, then ignore the copy error: a truncated upstream body was
// billed as a successful generation while the customer received a broken 200.
func TestMultimodalUpstreamReadErrorIsNotCharged(t *testing.T) {
	const modelName = "edge-read-error-model"
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: modelName, GroupName: "default", FixedPrice: 0.25, Currency: "CNY",
	})

	previousClient := provider.SharedDefaultHTTPClient
	provider.SharedDefaultHTTPClient = &http.Client{Transport: auditReviewTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       failingReadCloser{err: errors.New("connection reset by peer")},
		}, nil
	})}
	t.Cleanup(func() { provider.SharedDefaultHTTPClient = previousClient })

	handler := NewMultimodalHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "edge-read-error", BaseURL: "https://image.invalid", Models: []string{modelName},
		Protocols: []string{"images"}, Status: "active", Weight: 1,
	}}), repo)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"`+modelName+`","prompt":"x"}`))
	handler.dispatchAndForward(c, &router.UpstreamRequest{
		Path: "/v1/images/generations", Method: http.MethodPost,
		Body: []byte(`{"model":"` + modelName + `","prompt":"x"}`), ContentType: "application/json",
		Model: modelName, Protocol: "images", BillableUnits: 1,
	}, "sess-read-error")

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when the upstream body cannot be read", w.Code)
	}
	if !strings.Contains(w.Body.String(), "upstream_error") {
		t.Fatalf("error response does not identify the upstream failure: %s", w.Body.String())
	}
	for _, row := range edgeUsageRows(t, repo, 1) {
		if row.StatusCode == http.StatusOK {
			t.Fatalf("a body read failure was audited as a 200 success: %+v", row)
		}
		if row.Cost != 0 {
			t.Fatalf("a body read failure was billed %v", row.Cost)
		}
	}
}

// A GET video-task query never bills. The read-before-billing change must not
// turn into a charge or a status change for it.
func TestMultimodalGetReadStaysUnbilled(t *testing.T) {
	const modelName = "edge-task-model"
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: modelName, GroupName: "default", FixedPrice: 0.25, Currency: "CNY",
	})

	previousClient := provider.SharedDefaultHTTPClient
	provider.SharedDefaultHTTPClient = &http.Client{Transport: auditReviewTransport(func(r *http.Request) (*http.Response, error) {
		return auditReviewResponse(http.StatusOK, `{"id":"up-stream-task","status":"queued"}`), nil
	})}
	t.Cleanup(func() { provider.SharedDefaultHTTPClient = previousClient })

	handler := NewMultimodalHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "edge-task", BaseURL: "https://video.invalid", Models: []string{modelName},
		Protocols: []string{"videos"}, Status: "active", Weight: 1,
	}}), repo)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/tasks/up-stream-task", nil)
	handler.dispatchAndForward(c, &router.UpstreamRequest{
		Path: "/v1/videos/tasks/up-stream-task", Method: http.MethodGet,
		Model: modelName, Protocol: "videos",
	}, "sess-task")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, "up-stream-task") {
		t.Fatalf("body = %q, want the buffered upstream payload", got)
	}
	rows := edgeUsageRows(t, repo, 1)
	if rows[0].StatusCode != http.StatusOK || rows[0].Cost != 0 {
		t.Fatalf("GET query row = status %d cost %v, want an unbilled 200", rows[0].StatusCode, rows[0].Cost)
	}
}

func TestReadMultimodalBodyRejectsOversizedResponse(t *testing.T) {
	if _, err := readMultimodalBody(strings.NewReader(strings.Repeat("x", 65)), 64); err == nil {
		t.Fatal("an upstream body above the buffer limit must be rejected")
	}
	body, err := readMultimodalBody(strings.NewReader("ok"), 64)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body = %q err = %v, want the body within the limit", body, err)
	}
}
