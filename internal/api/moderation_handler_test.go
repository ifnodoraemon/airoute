package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func moderationContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/moderations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// Without a moderation-capable channel the gateway must refuse, not fabricate a
// verdict: a synthetic "unflagged" for every input silently defeats downstream
// content filtering and looks like a normal 200 response.
func TestModerationsWithoutChannelIsRejected(t *testing.T) {
	h := NewHandler(router.NewDispatcher(nil))
	c, w := moderationContext(t, `{"input":"I want to hurt someone"}`)

	h.HandleModerations(c)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d (no synthetic verdict may be returned)", w.Code, http.StatusNotImplemented)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, w.Body.String())
	}
	if body.Error.Code != "moderation_not_configured" {
		t.Fatalf("error code = %q, want moderation_not_configured", body.Error.Code)
	}
	if strings.Contains(w.Body.String(), "flagged") {
		t.Fatal("the rejection response must not contain a moderation verdict")
	}
}

// The synthetic response remains available, but only when explicitly enabled for
// local development.
func TestModerationsStubRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("GATEWAY_MODERATION_STUB", "true")
	h := NewHandler(router.NewDispatcher(nil))
	c, w := moderationContext(t, `{"input":"hello"}`)

	h.HandleModerations(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the stub enabled", w.Code)
	}
	var resp ModerationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("stub response is not JSON: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Flagged {
		t.Fatalf("unexpected stub response: %+v", resp)
	}
}
