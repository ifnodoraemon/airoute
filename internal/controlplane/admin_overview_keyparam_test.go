package controlplane

import (
	"net/http"
	"net/url"
	"testing"
)

// The legacy `key` drill-down parameter must mean one thing for every role and
// every stats endpoint.
//
// It used to be compared byte-for-byte against digests on the non-admin path
// (so only a raw digest worked) while the admin path re-hashed whatever it
// received (so a plaintext credential matched and an `ak_…` identifier hashed
// into a digest that matched no row, silently reporting zero traffic).
func TestStatsKeyParamResolvesPublicIdentifier(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}

	if fixture.aliceKey.PublicID == "" {
		t.Fatal("fixture key has no public identifier")
	}
	byPublicID := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, admin,
		"/api/v1/admin/stats/overview?key="+url.QueryEscape(fixture.aliceKey.PublicID))
	if byPublicID.Code != http.StatusOK {
		t.Fatalf("overview by ak_ id = %d: %s", byPublicID.Code, byPublicID.Body.String())
	}
	requests, tokens := statsOverviewNumbers(t, byPublicID)
	if requests != 1 || tokens != 10 {
		t.Fatalf("overview by ak_ id = %v requests / %v tokens, want alice's 1/10", requests, tokens)
	}

	series := statsRequest(fixture.handler, fixture.handler.GetStatsTimeSeries, admin,
		"/api/v1/admin/stats/timeseries?timeframe=24h&key="+url.QueryEscape(fixture.aliceKey.PublicID))
	if series.Code != http.StatusOK {
		t.Fatalf("timeseries by ak_ id = %d: %s", series.Code, series.Body.String())
	}
	if got := statsSeriesRequests(t, series); got != 1 {
		t.Fatalf("timeseries by ak_ id = %v requests, want 1", got)
	}
}

// A non-admin may use the public identifier too, and is still scoped to its own
// keys.
func TestStatsKeyParamEnforcesOwnershipForPublicIdentifier(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	alice := &AdminClaims{Username: fixture.alice.Username, Role: "user"}

	own := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, alice,
		"/api/v1/admin/stats/overview?key="+url.QueryEscape(fixture.aliceKey.PublicID))
	if own.Code != http.StatusOK {
		t.Fatalf("own key by ak_ id = %d: %s", own.Code, own.Body.String())
	}
	if requests, _ := statsOverviewNumbers(t, own); requests != 1 {
		t.Fatalf("own key by ak_ id = %v requests, want 1", requests)
	}

	foreign := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, alice,
		"/api/v1/admin/stats/overview?key="+url.QueryEscape(fixture.bobKey.PublicID))
	if foreign.Code != http.StatusForbidden {
		t.Fatalf("another user's key by ak_ id = %d, want 403: %s", foreign.Code, foreign.Body.String())
	}
}

// A value that is neither a digest nor an `ak_…` identifier is a client error,
// never a plausible-looking zero.
func TestStatsKeyParamRejectsUnresolvableReferences(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}

	cases := []struct {
		name string
		key  string
	}{
		{"plaintext credential", "sk-stats-alice"},
		{"display mask", "sk-st***lice"},
		{"unknown public identifier", "ak_00000000000000000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, admin,
				"/api/v1/admin/stats/overview?key="+url.QueryEscape(tc.key))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			envelope := decodeEnvelope(t, w)
			if envelope["error"] == nil {
				t.Errorf("refusal payload has no error message: %s", w.Body.String())
			}
		})
	}
}

// A raw digest stays accepted for existing scripted callers; the console itself
// only sends ak_… or key_id.
func TestStatsKeyParamAcceptsRawDigest(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}

	w := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, admin,
		"/api/v1/admin/stats/overview?key="+url.QueryEscape(fixture.aliceKey.Key))
	if w.Code != http.StatusOK {
		t.Fatalf("overview by digest = %d: %s", w.Code, w.Body.String())
	}
	if requests, _ := statsOverviewNumbers(t, w); requests != 1 {
		t.Fatalf("overview by digest = %v requests, want 1", requests)
	}
}
