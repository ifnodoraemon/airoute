package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type statsKeyIDFixture struct {
	handler  *AdminHandler
	repo     *storage.Repository
	alice    *storage.UserRecord
	bob      *storage.UserRecord
	aliceKey *storage.APIKeyRecord
	bobKey   *storage.APIKeyRecord
}

func newStatsKeyIDFixture(t *testing.T) *statsKeyIDFixture {
	t.Helper()
	handler, repo := regressionHandler(t)
	fixture := &statsKeyIDFixture{handler: handler, repo: repo}

	fixture.alice = &storage.UserRecord{
		Username: "stats-alice", Email: "stats-alice@example.invalid", PasswordHash: "synthetic",
		Role: "user", Status: "active", Balance: 100, GroupName: "default",
	}
	if err := repo.CreateUser(fixture.alice); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	fixture.bob = &storage.UserRecord{
		Username: "stats-bob", Email: "stats-bob@example.invalid", PasswordHash: "synthetic",
		Role: "user", Status: "active", Balance: 100, GroupName: "default",
	}
	if err := repo.CreateUser(fixture.bob); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	fixture.aliceKey = &storage.APIKeyRecord{Key: "sk-stats-alice", TenantID: fixture.alice.Username, UserID: fixture.alice.ID, AllowedModels: []string{"*"}, Status: "active"}
	if err := repo.CreateAPIKey(fixture.aliceKey); err != nil {
		t.Fatalf("create alice key: %v", err)
	}
	fixture.bobKey = &storage.APIKeyRecord{Key: "sk-stats-bob", TenantID: fixture.bob.Username, UserID: fixture.bob.ID, AllowedModels: []string{"*"}, Status: "active"}
	if err := repo.CreateAPIKey(fixture.bobKey); err != nil {
		t.Fatalf("create bob key: %v", err)
	}

	recordStatsUsage(t, repo, fixture.aliceKey, fixture.alice.Username, "stats-alice-model", 10)
	recordStatsUsage(t, repo, fixture.bobKey, fixture.bob.Username, "stats-bob-model", 20)
	return fixture
}

func recordStatsUsage(t *testing.T, repo *storage.Repository, key *storage.APIKeyRecord, tenant, model string, tokens int) {
	t.Helper()
	if err := repo.RecordUsageLog(&storage.UsageLogRecord{
		APIKey: key.Key, TenantID: tenant, Model: model, Channel: "stats-synthetic",
		Cost: 0.5, TotalTokens: tokens, StatusCode: 200,
	}); err != nil {
		t.Fatalf("record usage for %s: %v", model, err)
	}
}

func statsRequest(handler *AdminHandler, fn gin.HandlerFunc, claims *AdminClaims, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if claims != nil {
		c.Set("admin_claims", claims)
	}
	fn(c)
	return w
}

func statsOverviewNumbers(t *testing.T, w *httptest.ResponseRecorder) (requests, tokens float64) {
	t.Helper()
	envelope := decodeEnvelope(t, w)
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("overview response has no data object: %s", w.Body.String())
	}
	requests, _ = data["total_requests"].(float64)
	tokens, _ = data["total_tokens"].(float64)
	return requests, tokens
}

func statsSeriesRequests(t *testing.T, w *httptest.ResponseRecorder) float64 {
	t.Helper()
	envelope := decodeEnvelope(t, w)
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("timeseries response has no data object: %s", w.Body.String())
	}
	buckets, _ := data["buckets"].([]any)
	var total float64
	for _, bucket := range buckets {
		asMap, ok := bucket.(map[string]any)
		if !ok {
			continue
		}
		if requests, ok := asMap["requests"].(float64); ok {
			total += requests
		}
	}
	return total
}

func overviewPath(keyID int64) string {
	return fmt.Sprintf("/api/v1/admin/stats/overview?key_id=%d", keyID)
}

func timeSeriesPath(keyID int64) string {
	return fmt.Sprintf("/api/v1/admin/stats/timeseries?timeframe=24h&key_id=%d", keyID)
}

// #6: the console sends the numeric key id because the key list only exposes a
// masked value. The handlers must resolve it to the stored digest before asking
// the repository, otherwise usage_logs lookups match nothing and a busy key shows
// zero traffic.
func TestStatsKeyIDResolvesToStoredDigest(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}

	overview := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, admin, overviewPath(fixture.aliceKey.ID))
	if overview.Code != http.StatusOK {
		t.Fatalf("overview by key_id = %d: %s", overview.Code, overview.Body.String())
	}
	requests, tokens := statsOverviewNumbers(t, overview)
	if requests != 1 || tokens != 10 {
		t.Fatalf("overview by key_id = %v requests / %v tokens, want 1/10 (alice's traffic)", requests, tokens)
	}

	// The same numbers are produced by the legacy digest filter, which proves the
	// id was resolved to the digest rather than hashed as a client string.
	legacy := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, admin,
		"/api/v1/admin/stats/overview?key="+url.QueryEscape(fixture.aliceKey.Key))
	if legacy.Code != http.StatusOK {
		t.Fatalf("overview by digest = %d: %s", legacy.Code, legacy.Body.String())
	}
	legacyRequests, legacyTokens := statsOverviewNumbers(t, legacy)
	if legacyRequests != requests || legacyTokens != tokens {
		t.Fatalf("key_id produced %v/%v but the digest filter produced %v/%v", requests, tokens, legacyRequests, legacyTokens)
	}

	series := statsRequest(fixture.handler, fixture.handler.GetStatsTimeSeries, admin, timeSeriesPath(fixture.aliceKey.ID))
	if series.Code != http.StatusOK {
		t.Fatalf("timeseries by key_id = %d: %s", series.Code, series.Body.String())
	}
	if got := statsSeriesRequests(t, series); got != 1 {
		t.Fatalf("timeseries requests = %v, want 1", got)
	}
}

// #6: a non-admin may only drill into a key they own; the ownership decision is
// made on the resolved row.
func TestStatsKeyIDEnforcesNonAdminOwnership(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	alice := &AdminClaims{Username: fixture.alice.Username, Role: "user"}

	own := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, alice, overviewPath(fixture.aliceKey.ID))
	if own.Code != http.StatusOK {
		t.Fatalf("own key overview = %d: %s", own.Code, own.Body.String())
	}
	if requests, tokens := statsOverviewNumbers(t, own); requests != 1 || tokens != 10 {
		t.Fatalf("own key overview = %v/%v, want 1/10", requests, tokens)
	}
	if ownSeries := statsRequest(fixture.handler, fixture.handler.GetStatsTimeSeries, alice, timeSeriesPath(fixture.aliceKey.ID)); ownSeries.Code != http.StatusOK {
		t.Fatalf("own key timeseries = %d: %s", ownSeries.Code, ownSeries.Body.String())
	}

	foreign := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, alice, overviewPath(fixture.bobKey.ID))
	if foreign.Code != http.StatusForbidden {
		t.Fatalf("another user's key overview = %d, want 403: %s", foreign.Code, foreign.Body.String())
	}
	if envelope := decodeEnvelope(t, foreign); envelope["code"] != float64(403) {
		t.Fatalf("refusal payload = %v, want code 403", envelope["code"])
	}

	foreignSeries := statsRequest(fixture.handler, fixture.handler.GetStatsTimeSeries, alice, timeSeriesPath(fixture.bobKey.ID))
	if foreignSeries.Code != http.StatusForbidden {
		t.Fatalf("another user's key timeseries = %d, want 403: %s", foreignSeries.Code, foreignSeries.Body.String())
	}

	// Without a filter the user still sees only their own aggregate.
	all := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, alice, "/api/v1/admin/stats/overview")
	if all.Code != http.StatusOK {
		t.Fatalf("unfiltered overview = %d: %s", all.Code, all.Body.String())
	}
	if requests, _ := statsOverviewNumbers(t, all); requests != 1 {
		t.Fatalf("unfiltered user overview = %v requests, want 1 (own only)", requests)
	}

	// The administrator still sees the whole cluster without a filter.
	adminAll := statsRequest(fixture.handler, fixture.handler.GetStatsOverview, &AdminClaims{Username: "admin", Role: "admin"},
		"/api/v1/admin/stats/overview")
	if requests, _ := statsOverviewNumbers(t, adminAll); requests != 2 {
		t.Fatalf("unfiltered admin overview = %v requests, want 2", requests)
	}
}

// #6: a malformed or unknown id is a client error, not a silent full-cluster read.
func TestStatsKeyIDValidation(t *testing.T) {
	fixture := newStatsKeyIDFixture(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}

	badRequests := []struct {
		name string
		fn   gin.HandlerFunc
		path string
	}{
		{"overview malformed id", fixture.handler.GetStatsOverview, "/api/v1/admin/stats/overview?key_id=abc"},
		{"overview non-positive id", fixture.handler.GetStatsOverview, "/api/v1/admin/stats/overview?key_id=0"},
		{"timeseries malformed id", fixture.handler.GetStatsTimeSeries, "/api/v1/admin/stats/timeseries?timeframe=24h&key_id=abc"},
	}
	for _, tc := range badRequests {
		w := statsRequest(fixture.handler, tc.fn, admin, tc.path)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400: %s", tc.name, w.Code, w.Body.String())
		}
	}

	notFound := []struct {
		name string
		fn   gin.HandlerFunc
		path string
	}{
		{"overview unknown id", fixture.handler.GetStatsOverview, "/api/v1/admin/stats/overview?key_id=987654"},
		{"timeseries unknown id", fixture.handler.GetStatsTimeSeries, "/api/v1/admin/stats/timeseries?timeframe=24h&key_id=987654"},
	}
	for _, tc := range notFound {
		w := statsRequest(fixture.handler, tc.fn, admin, tc.path)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404: %s", tc.name, w.Code, w.Body.String())
		}
	}
}
