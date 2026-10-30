package controlplane

// Regression suite for the control-plane / permission-boundary fixes of the
// 2026-10-29 hunt (docs/audit/2026-10-29-bughunt/01-controlplane.md §6).
//
// These tests used to be bug probes asserting the defective behaviour; they are
// now flipped to assert the corrected contract, so a regression fails here instead
// of silently reintroducing the defect.
//
//	export PATH=$PATH:/usr/local/go/bin
//	go test ./internal/controlplane/ -run 'TestKeyAdminRevocation|TestBatchStatus|TestUsernameIdentity|TestSupplierCostCSV|TestAuthEndpointsReject' -v

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// --- CP-01: an operator's revocation is durable ---------------------------

func newHardeningHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func invokeKeyUpdateAs(t *testing.T, handler *AdminHandler, keyID int64, claims *AdminClaims, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/keys/1", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: int64String(keyID)}}
	if claims != nil {
		c.Set("admin_claims", claims)
	}
	handler.UpdateAPIKey(c)
	return w
}

func invokeKeyBatchStatusAs(t *testing.T, handler *AdminHandler, claims *AdminClaims, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/batch-status", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if claims != nil {
		c.Set("admin_claims", claims)
	}
	handler.BatchStatusAPIKeys(c)
	return w
}

// seedUserAndKey creates an account plus one active key owned by it.
func seedUserAndKey(t *testing.T, repo *storage.Repository, username string) (*storage.UserRecord, *storage.APIKeyRecord) {
	t.Helper()
	user := &storage.UserRecord{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: "x",
		Role:         "user",
		Status:       "active",
		Balance:      10,
		GroupName:    "default",
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	key := &storage.APIKeyRecord{
		Key:           "sk-airoute-" + strings.Repeat("a", 32),
		TenantID:      username,
		UserID:        user.ID,
		RPM:           60,
		TPM:           100000,
		Budget:        100,
		GroupName:     "default",
		Status:        "active",
		AllowedModels: []string{"*"},
	}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key for %s: %v", username, err)
	}
	return user, key
}

func TestKeyAdminRevocationCannotBeUndoneByOwner(t *testing.T) {
	handler, repo := newHardeningHandler(t)
	user, key := seedUserAndKey(t, repo, "tenant-alice")

	adminClaims := &AdminClaims{Username: "root", Role: "admin"}
	ownerClaims := &AdminClaims{Username: user.Username, Role: "user"}

	// 1. The operator revokes the key.
	w := invokeKeyUpdateAs(t, handler, key.ID, adminClaims, `{"status":"disabled"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("admin disable: status=%d body=%s", w.Code, w.Body.String())
	}
	stored, err := repo.GetAPIKey(key.ID)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if stored.Status != "disabled" || !stored.AdminDisabled {
		t.Fatalf("after the admin disable: status=%q admin_disabled=%v, want disabled/true", stored.Status, stored.AdminDisabled)
	}
	if middleware.KeyStatusDenial(stored.Status) == nil {
		t.Fatalf("the data plane would still admit the revoked key")
	}

	// 2. The owner tries to undo it: refused, and nothing changed.
	w = invokeKeyUpdateAs(t, handler, key.ID, ownerClaims, `{"status":"active"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("owner re-enable: status=%d, want 403 (body: %s)", w.Code, w.Body.String())
	}
	stored, _ = repo.GetAPIKey(key.ID)
	if stored.Status != "disabled" || !stored.AdminDisabled {
		t.Fatalf("owner re-enable leaked through: status=%q admin_disabled=%v", stored.Status, stored.AdminDisabled)
	}
	if middleware.KeyStatusDenial(stored.Status) == nil {
		t.Fatalf("the revoked key became usable again")
	}

	// 3. An administrator may lift the revocation; the flag must clear so the owner
	//    regains self-service control.
	if w := invokeKeyUpdateAs(t, handler, key.ID, adminClaims, `{"status":"active"}`); w.Code != http.StatusOK {
		t.Fatalf("admin re-enable: status=%d body=%s", w.Code, w.Body.String())
	}
	stored, _ = repo.GetAPIKey(key.ID)
	if stored.Status != "active" || stored.AdminDisabled {
		t.Fatalf("after the admin re-enable: status=%q admin_disabled=%v, want active/false", stored.Status, stored.AdminDisabled)
	}

	// 4. Self-service active <-> disabled still works on a key the operator has not
	//    revoked.
	if w := invokeKeyUpdateAs(t, handler, key.ID, ownerClaims, `{"status":"disabled"}`); w.Code != http.StatusOK {
		t.Fatalf("owner self-disable: status=%d body=%s", w.Code, w.Body.String())
	}
	stored, _ = repo.GetAPIKey(key.ID)
	if stored.Status != "disabled" || stored.AdminDisabled {
		t.Fatalf("owner self-disable: status=%q admin_disabled=%v, want disabled/false", stored.Status, stored.AdminDisabled)
	}
	if w := invokeKeyUpdateAs(t, handler, key.ID, ownerClaims, `{"status":"active"}`); w.Code != http.StatusOK {
		t.Fatalf("owner self-enable: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestKeyAdminRevocationSurvivesBatchStatus(t *testing.T) {
	handler, repo := newHardeningHandler(t)
	user, key := seedUserAndKey(t, repo, "tenant-bob")

	adminClaims := &AdminClaims{Username: "root", Role: "admin"}
	ownerClaims := &AdminClaims{Username: user.Username, Role: "user"}

	// The operator's bulk revocation must record the flag (otherwise the tenant can
	// undo it key by key).
	w := invokeKeyBatchStatusAs(t, handler, adminClaims,
		`{"ids":[`+int64String(key.ID)+`],"status":"disabled"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("admin batch disable: status=%d body=%s", w.Code, w.Body.String())
	}
	stored, _ := repo.GetAPIKey(key.ID)
	if stored.Status != "disabled" || !stored.AdminDisabled {
		t.Fatalf("admin batch disable: status=%q admin_disabled=%v", stored.Status, stored.AdminDisabled)
	}

	// The tenant's bulk enable is refused and changes nothing.
	w = invokeKeyBatchStatusAs(t, handler, ownerClaims,
		`{"ids":[`+int64String(key.ID)+`],"status":"active"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("owner batch enable: status=%d, want 403 (body: %s)", w.Code, w.Body.String())
	}
	stored, _ = repo.GetAPIKey(key.ID)
	if stored.Status != "disabled" || !stored.AdminDisabled {
		t.Fatalf("owner batch enable leaked through: status=%q admin_disabled=%v", stored.Status, stored.AdminDisabled)
	}

	// The administrator can lift it in bulk too.
	if w := invokeKeyBatchStatusAs(t, handler, adminClaims,
		`{"ids":[`+int64String(key.ID)+`],"status":"active"}`); w.Code != http.StatusOK {
		t.Fatalf("admin batch enable: status=%d body=%s", w.Code, w.Body.String())
	}
	stored, _ = repo.GetAPIKey(key.ID)
	if stored.Status != "active" || stored.AdminDisabled {
		t.Fatalf("admin batch enable: status=%q admin_disabled=%v, want active/false", stored.Status, stored.AdminDisabled)
	}
}

func TestKeyCreationRecordsAdminDisabledAndExposesIt(t *testing.T) {
	handler, repo := newHardeningHandler(t)
	adminClaims := &AdminClaims{Username: "root", Role: "admin"}

	// An operator issuing an already-disabled key is making a revocation decision.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys", strings.NewReader(`{"status":"disabled","tenant_id":"t1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("admin_claims", adminClaims)
	handler.CreateAPIKey(c)
	if w.Code != http.StatusOK {
		t.Fatalf("admin create disabled key: status=%d body=%s", w.Code, w.Body.String())
	}
	keys, err := repo.ListAPIKeys()
	if err != nil || len(keys) == 0 {
		t.Fatalf("list keys: %v (%d)", err, len(keys))
	}
	if !keys[0].AdminDisabled {
		t.Fatalf("an operator-created disabled key was not flagged admin_disabled")
	}

	// A case-variant status from an operator must still be recognised as a
	// revocation (otherwise the flag would stay false and the owner could re-enable
	// the key).
	for _, variant := range []string{"DISABLED", " Disabled "} {
		rec := ApplyKeyUpdate(KeyAccess{Allowed: true, IsAdmin: true},
			&storage.APIKeyRecord{ID: 1, Status: "active"},
			UpdateAPIKeyRequest{Status: ptrString(variant)})
		if !rec.Allowed || rec.Record.Status != "disabled" || !rec.Record.AdminDisabled {
			t.Fatalf("admin status %q: %+v", variant, rec)
		}
	}
	// An unknown creation status is a client error, not a stored value.
	if outcome := (&KeyCreationRules{}).PlanCreate(adminClaims,
		CreateAPIKeyRequest{Status: "nonsense"}); outcome.Allowed || outcome.Denial != KeyCreationInvalidValue {
		t.Fatalf("PlanCreate accepted an unknown status: %+v", outcome)
	}

	// The wire view exposes the flag so the console can distinguish the two disable
	// reasons without a second request.
	view := apiKeyViewFrom(keys[0])
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	if !strings.Contains(string(raw), `"admin_disabled":true`) {
		t.Fatalf("the key view does not expose admin_disabled: %s", raw)
	}
}

// --- CP-02: account identity is case-insensitive --------------------------

func TestUsernameIdentityIsCaseInsensitive(t *testing.T) {
	handler, repo := newHardeningHandler(t)

	victim := &storage.UserRecord{
		Username:     "victim",
		Email:        "victim@example.com",
		PasswordHash: "x",
		Role:         "user",
		Status:       "active",
		GroupName:    "default",
	}
	if err := repo.CreateUser(victim); err != nil {
		t.Fatalf("create victim: %v", err)
	}

	// 1. The database refuses a case variant: this is what stops an attacker from
	//    owning the case-variant account and resetting the victim's lockout bucket.
	err := repo.CreateUser(&storage.UserRecord{
		Username:     "VICTIM",
		Email:        "decoy@example.com",
		PasswordHash: "x",
		Role:         "user",
		Status:       "active",
		GroupName:    "default",
	})
	if err == nil {
		t.Fatalf("a case-variant account was created next to the existing one")
	}
	if !errors.Is(err, storage.ErrUsernameAlreadyRegistered) {
		t.Fatalf("creating the case variant failed with %v, want ErrUsernameAlreadyRegistered", err)
	}

	// 2. Lookups agree with the uniqueness rule.
	found, err := repo.GetUserByUsername("VICTIM")
	if err != nil || found == nil || found.Username != "victim" {
		t.Fatalf("GetUserByUsername(VICTIM) = %+v, %v; want the victim account", found, err)
	}

	// 3. The unique index really exists (the database is the authority).
	var indexCount int
	if err := repo.GetDB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'uq_users_username_lower'`).Scan(&indexCount); err != nil {
		t.Fatalf("inspect sqlite schema: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("uq_users_username_lower is missing (count=%d)", indexCount)
	}

	// 4. Registration reports the conflict instead of creating the decoy.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"username":"VICTIM","email":"decoy@example.com","password":"secret123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "203.0.113.77:1234"
	handler.Register(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("registering VICTIM next to victim: status=%d, want 400 (body: %s)", w.Code, w.Body.String())
	}
	if n, err := repo.GetUserByUsername("VICTIM"); err != nil || n == nil || n.ID != victim.ID {
		t.Fatalf("the decoy account replaced or shadowed the victim: %+v, %v", n, err)
	}

	// 5. The throttle's account bucket therefore only ever describes one account:
	//    logging in as "VICTIM" proves the password of "victim", so a success
	//    resets that same account's counter (the intended B-02 behaviour) and there
	//    is no second account whose login could be used to reset it.
	a := accountBuckets(loginClient{Username: "VICTIM"})
	b := accountBuckets(loginClient{Username: "victim"})
	if len(a) != 1 || len(b) != 1 || a[0].subject != b[0].subject {
		t.Fatalf("case variants do not share one account bucket: %+v vs %+v", a, b)
	}
}

// --- CP-03: an invalid status is an error, never a default -----------------

func TestBatchStatusRejectsInvalidStatus(t *testing.T) {
	handler, _ := newHardeningHandler(t)

	// The batch endpoint must refuse, not enable. This is the difference between a
	// failed bulk revocation and a silent mass re-enable.
	for _, bad := range []string{"disable", "off", "Disable", "0", ""} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/batch-status",
			strings.NewReader(`{"ids":[1],"status":"`+bad+`"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		if req, ok := handler.bindBatchStatus(c); ok || req != nil {
			t.Fatalf("bindBatchStatus accepted status %q", bad)
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("bindBatchStatus(%q) = %d, want 400", bad, w.Code)
		}
	}
	for _, good := range []string{"active", "disabled"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/batch-status",
			strings.NewReader(`{"ids":[1],"status":"`+good+`"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		req, ok := handler.bindBatchStatus(c)
		if !ok || req == nil || req.Status != good {
			t.Fatalf("bindBatchStatus(%q) was refused: %+v %d", good, req, w.Code)
		}
	}

	// The single-key update path validates the same enum (otherwise the batch
	// endpoint would be strict while one PATCH still stored nonsense).
	owner := &storage.UserRecord{ID: 1, Username: "owner", GroupName: "default"}
	access := KeyAccess{Allowed: true, User: owner}
	outcome := ApplyKeyUpdate(access, &storage.APIKeyRecord{ID: 5, Status: "active", GroupName: "default"},
		UpdateAPIKeyRequest{Status: ptrString("bogus")})
	if outcome.Allowed || outcome.Denial != KeyUpdateInvalidValue {
		t.Fatalf("ApplyKeyUpdate accepted an unknown status: %+v", outcome)
	}

	// Channel side: the coercion helper no longer invents "active".
	for _, bad := range []string{"disable", "off", "bogus", ""} {
		if got := normalizeChannelStatus(bad); got != "" {
			t.Fatalf("normalizeChannelStatus(%q) = %q, want it refused", bad, got)
		}
	}
	if got := normalizeChannelStatus("disabled"); got != "disabled" {
		t.Fatalf("normalizeChannelStatus(disabled) = %q", got)
	}

	// A channel update with an unknown status is rejected instead of being stored
	// (the data plane treats anything but active as disabled, so storing it would
	// silently take the channel offline).
	existing := &storage.ChannelRecord{Name: "c", BaseURL: "http://c.invalid", Type: "openai", Status: "active"}
	if err := mergeChannelUpdate(existing, &channelPatch{Status: "nonsense"}); err == nil {
		t.Fatalf("mergeChannelUpdate stored an unknown status")
	}
	if existing.Status != "active" {
		t.Fatalf("mergeChannelUpdate modified the channel on a refused update: %q", existing.Status)
	}
}

// ptrString returns a pointer to v, for the pointer-based update request.
func ptrString(v string) *string { return &v }

// --- CP-04: exported CSV neutralises spreadsheet formulas ------------------

func TestSupplierCostCSVExportNeutralisesFormulas(t *testing.T) {
	handler, repo := newHardeningHandler(t)

	formulas := []string{
		`=cmd|' /C calc'!A0`,
		`@SUM(1+1)`,
		`+41`,
		"-41",
		"\t=cmd|' /C calc'!A0",
		" =cmd|' /C calc'!A0",
	}
	for i, channel := range formulas {
		rec := &storage.SupplierCostRecord{
			Channel:                   channel,
			Model:                     "m-" + int64String(int64(i)),
			InputCostUnitsPerMillion:  storage.ToUnits(1),
			OutputCostUnitsPerMillion: storage.ToUnits(2),
			Status:                    "active",
		}
		if err := repo.UpsertSupplierCost(rec); err != nil {
			t.Fatalf("seed supplier cost %q: %v", channel, err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/supplier-costs/export", nil)
	handler.ExportSupplierCosts(c)
	if w.Code != http.StatusOK {
		t.Fatalf("export status=%d", w.Code)
	}

	body := strings.TrimPrefix(w.Body.String(), "\ufeff")
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != len(formulas)+1 {
		t.Fatalf("expected %d data rows, got %d: %q", len(formulas), len(lines)-1, body)
	}
	for _, line := range lines[1:] {
		line = strings.TrimSuffix(line, "\r")
		// Every dangerous cell must be neutralised by a leading apostrophe. A cell
		// that starts with an apostrophe keeps its original text after it, so the
		// check is "the first character of the CSV cell is '".
		if !strings.HasPrefix(line, "'") {
			t.Errorf("formula cell was not neutralised: %q", line)
		}
	}

	// A benign value must be left exactly as-is (no over-quoting), and the numeric
	// columns must stay machine-readable.
	if err := repo.UpsertSupplierCost(&storage.SupplierCostRecord{
		Channel: "ch-benign", Model: "m-benign",
		InputCostUnitsPerMillion: storage.ToUnits(1), OutputCostUnitsPerMillion: storage.ToUnits(2),
		Status: "active",
	}); err != nil {
		t.Fatalf("seed benign cost: %v", err)
	}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/supplier-costs/export", nil)
	handler.ExportSupplierCosts(c)
	if !strings.Contains(w.Body.String(), "\nch-benign,m-benign,1.0000,2.0000,active") {
		t.Fatalf("benign row was mangled: %q", w.Body.String())
	}

	// The helper itself: leading whitespace then a formula character is still a
	// formula to a spreadsheet.
	for _, tc := range []struct{ in, want string }{
		{"=1+1", "'=1+1"},
		{" =1+1", "' =1+1"},
		{"\t@x", "'\t@x"},
		{"-5", "'-5"},
		{"plain", "plain"},
		{"", ""},
		{"  ", "  "},
	} {
		if got := sanitizeCSVCell(tc.in); got != tc.want {
			t.Errorf("sanitizeCSVCell(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- CP-05: the unauthenticated auth endpoints reject cross-site requests ---

func TestAuthEndpointsRejectCrossSiteRequests(t *testing.T) {
	handler, repo := newHardeningHandler(t)
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("GATEWAY_PUBLIC_URL", "")

	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := repo.CreateUser(&storage.UserRecord{
		Username:     "siteuser",
		Email:        "siteuser@example.com",
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		Balance:      1,
		GroupName:    "default",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	login := func(contentType, origin, referer string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/auth/login",
			strings.NewReader(`{"username":"siteuser","password":"secret123"}`))
		c.Request.Host = "gateway.local"
		if contentType != "" {
			c.Request.Header.Set("Content-Type", contentType)
		}
		if origin != "" {
			c.Request.Header.Set("Origin", origin)
		}
		if referer != "" {
			c.Request.Header.Set("Referer", referer)
		}
		handler.Login(c)
		return w
	}

	// A cross-site form cannot send a JSON content type without a preflight.
	if w := login("text/plain;charset=UTF-8", "https://evil.example", ""); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain login = %d, want 415 (body: %s)", w.Code, w.Body.String())
	}
	// A JSON request from another origin is refused even though the body was fine.
	if w := login("application/json", "https://evil.example", ""); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin login = %d, want 403 (body: %s)", w.Code, w.Body.String())
	}
	// Referer is the fallback when Origin is absent (older browsers, GET forms).
	if w := login("application/json", "", "https://evil.example/login"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin referer login = %d, want 403", w.Code)
	}
	// "Origin: null" (sandboxed iframe, file://) is cross-site and refused.
	if w := login("application/json", "null", ""); w.Code != http.StatusForbidden {
		t.Fatalf("Origin: null login = %d, want 403", w.Code)
	}
	// Same-origin browser traffic still works, with or without parameters.
	if w := login("application/json; charset=utf-8", "http://gateway.local", ""); w.Code != http.StatusOK {
		t.Fatalf("same-origin login = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	// CLI / scripted clients send no Origin at all and must keep working.
	if w := login("application/json", "", ""); w.Code != http.StatusOK {
		t.Fatalf("origin-less login = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	// Registration has the same gates.
	register := func(contentType, origin string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/auth/register",
			strings.NewReader(`{"username":"new-user","email":"new@example.com","password":"secret123"}`))
		c.Request.Host = "gateway.local"
		c.Request.RemoteAddr = "203.0.113.90:1234"
		if contentType != "" {
			c.Request.Header.Set("Content-Type", contentType)
		}
		if origin != "" {
			c.Request.Header.Set("Origin", origin)
		}
		handler.Register(c)
		return w
	}
	if w := register("text/plain", "https://evil.example"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain register = %d, want 415", w.Code)
	}
	if w := register("application/json", "https://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin register = %d, want 403", w.Code)
	}

	// Logout clears a cookie, so a cross-site POST could sign a victim out.
	logout := func(origin string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/auth/logout", nil)
		c.Request.Host = "gateway.local"
		if origin != "" {
			c.Request.Header.Set("Origin", origin)
		}
		handler.Logout(c)
		return w
	}
	if w := logout("https://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin logout = %d, want 403", w.Code)
	}
	if w := logout("http://gateway.local"); w.Code != http.StatusOK {
		t.Fatalf("same-origin logout = %d, want 200", w.Code)
	}
	if w := logout(""); w.Code != http.StatusOK {
		t.Fatalf("origin-less logout = %d, want 200", w.Code)
	}
}
