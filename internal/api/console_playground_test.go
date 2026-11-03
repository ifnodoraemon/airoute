package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type consoleFixture struct {
	engine                    *gin.Engine
	repo                      *storage.Repository
	user                      *storage.UserRecord
	key, otherKey             *storage.APIKeyRecord
	token, adminToken, secret string
	calls                     atomic.Int32
}

func newConsoleFixture(t *testing.T) *consoleFixture {
	t.Helper()
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "console-chat", GroupName: "default", PromptPrice: 2, CompletionPrice: 4, Currency: "CNY"})
	f := &consoleFixture{repo: repo}
	users := []*storage.UserRecord{
		{Username: "console-alice", Role: "user", Status: "active", Balance: 10},
		{Username: "console-bob", Role: "user", Status: "active", Balance: 10},
		{Username: "console-admin", Role: "admin", Status: "active"},
	}
	for _, user := range users {
		if err := repo.CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	f.user = users[0]
	var err error
	f.secret, err = controlplane.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	otherSecret, err := controlplane.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	f.key = &storage.APIKeyRecord{Key: f.secret, UserID: users[0].ID, TenantID: users[0].Username, Status: "active", AllowedModels: []string{"console-chat", "console-gemini"}}
	f.otherKey = &storage.APIKeyRecord{Key: otherSecret, UserID: users[1].ID, TenantID: users[1].Username, Status: "active", AllowedModels: []string{"console-chat"}}
	for _, key := range []*storage.APIKeyRecord{f.key, f.otherKey} {
		if err := repo.CreateAPIKey(key); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := repo.ToModelAPIKeys()
	if err != nil {
		t.Fatal(err)
	}
	previous := config.GetGlobalConfig()
	cfg := &config.Config{}
	cfg.UpdateAPIKeys(keys, true)
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		for _, name := range []string{"X-Airoute-Key-ID", "X-CSRF-Token", "Cookie", "x-admin-token"} {
			if r.Header.Get(name) != "" {
				t.Errorf("console credential/selection header %s leaked upstream", name)
			}
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.Path, ":generateContent") {
			fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"native console answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`)
			return
		}
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"console answer\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000,\"completion_tokens\":500,\"total_tokens\":1500}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"console-1","object":"chat.completion","model":"console-chat","choices":[{"index":0,"message":{"role":"assistant","content":"console answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"total_tokens":1500}}`)
	}))
	t.Cleanup(upstream.Close)
	dispatcher := router.NewDispatcher([]model.ChannelConfig{
		{Name: "console-openai", Type: model.ProviderOpenAI, BaseURL: upstream.URL + "/v1", APIKey: "synthetic-upstream", Models: []string{"console-chat"}},
		{Name: "console-google", Type: model.ProviderGemini, BaseURL: upstream.URL, APIKey: "synthetic-upstream", Models: []string{"console-gemini"}},
	})
	f.engine = SetupRouter(dispatcher, controlplane.NewAdminHandler(repo, nil, nil))
	f.token, err = controlplane.GenerateAdminToken(users[0], time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.adminToken, err = controlplane.GenerateAdminToken(users[2], time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *consoleFixture) request(method, path, token, keyID, csrf string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: controlplane.SessionCookieName, Value: token})
	}
	if keyID != "" {
		req.Header.Set("X-Airoute-Key-ID", keyID)
	}
	if csrf != "" {
		req.AddCookie(&http.Cookie{Name: controlplane.CSRFCookieName, Value: "console-csrf"})
		req.Header.Set(controlplane.CSRFHeaderName, csrf)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

const consoleChatBody = `{"model":"console-chat","messages":[{"role":"user","content":"hello"}],"max_tokens":1000}`

func TestConsolePlaygroundUsesSessionAndBillsSelectedKey(t *testing.T) {
	f := newConsoleFixture(t)
	id := strconv.FormatInt(f.key.ID, 10)
	for _, stream := range []bool{false, true} {
		body := strings.TrimSuffix(consoleChatBody, "}") + fmt.Sprintf(",\"stream\":%t}", stream)
		w := f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", f.token, id, "console-csrf", body)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "console answer") {
			t.Fatalf("stream=%v: %d %s", stream, w.Code, w.Body.String())
		}
		for _, secret := range []string{f.key.Key, f.secret, f.token} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("console response leaked a credential or digest")
			}
		}
	}
	logs := edgeUsageRows(t, f.repo, 2)
	for _, row := range logs {
		if row.APIKey != f.key.Key || row.TenantID != f.key.TenantID || math.Abs(row.Cost-0.004) > 1e-9 {
			t.Fatalf("incorrect selected-key attribution or bill: %+v", row)
		}
	}
	user, err := f.repo.GetUserByID(f.user.ID)
	if err != nil || math.Abs(user.Balance-9.992) > 1e-9 {
		t.Fatalf("wallet not settled: user=%+v error=%v", user, err)
	}
	key, err := f.repo.GetAPIKey(f.key.ID)
	if err != nil || key.UsedTokens != 3000 || math.Abs(key.UsedCost-0.008) > 1e-9 {
		t.Fatalf("selected key usage incorrect: key=%+v err=%v", key, err)
	}
	if f.calls.Load() != 2 {
		t.Fatalf("upstream calls=%d, want 2", f.calls.Load())
	}
}

func TestConsolePlaygroundRejectsUnauthorizedSelectionsBeforeUpstream(t *testing.T) {
	f := newConsoleFixture(t)
	id := strconv.FormatInt(f.key.ID, 10)
	for _, tc := range []struct {
		name, token, id, csrf string
		status                int
	}{
		{"no login", "", id, "console-csrf", 401},
		{"invalid login", "invalid-session", id, "console-csrf", 401},
		{"missing csrf", f.token, id, "", 403},
		{"invalid csrf", f.token, id, "incorrect", 403},
		{"missing selection", f.token, "", "console-csrf", 400},
		{"negative selection", f.token, "-1", "console-csrf", 400},
		{"public sentinel", f.token, "__none__", "console-csrf", 400},
		{"public identifier is not row id", f.token, f.key.PublicID, "console-csrf", 400},
		{"unknown selection", f.token, "9999999", "console-csrf", 403},
		{"other customer's key", f.token, strconv.FormatInt(f.otherKey.ID, 10), "console-csrf", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", tc.token, tc.id, tc.csrf, consoleChatBody)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
	if f.calls.Load() != 0 {
		t.Fatal("unauthorized selection reached upstream")
	}
	// A valid console cookie and key ID never authenticate the public SDK route.
	w := f.request("POST", "/v1/chat/completions", f.token, id, "console-csrf", consoleChatBody)
	if w.Code != 401 {
		t.Fatalf("SDK accepted a key ID: %d %s", w.Code, w.Body.String())
	}
	// Stored digests remain unusable as bearer credentials after the refactor.
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(consoleChatBody))
	req.Header.Set("Authorization", "Bearer "+f.key.Key)
	w = httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("SDK accepted stored digest: %d", w.Code)
	}
	// Empty runtime snapshots must never turn the console into open access.
	config.SetGlobalConfig(&config.Config{})
	w = f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", f.token, "", "console-csrf", consoleChatBody)
	if w.Code != 400 {
		t.Fatalf("console bypassed selection in open mode: %d %s", w.Code, w.Body.String())
	}
}

func TestConsolePlaygroundRejectsExpiredAndRevokedSessions(t *testing.T) {
	for _, scenario := range []string{"expired", "password changed", "deleted user"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConsoleFixture(t)
			token := f.token
			switch scenario {
			case "expired":
				var err error
				token, err = controlplane.GenerateAdminToken(f.user, -time.Hour)
				if err != nil {
					t.Fatal(err)
				}
			case "password changed":
				if _, err := f.repo.GetDB().Exec(`UPDATE users SET password_hash='changed' WHERE id=?`, f.user.ID); err != nil {
					t.Fatal(err)
				}
			case "deleted user":
				if _, err := f.repo.GetDB().Exec(`UPDATE users SET status='deleted' WHERE id=?`, f.user.ID); err != nil {
					t.Fatal(err)
				}
			}
			w := f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", token, strconv.FormatInt(f.key.ID, 10), "console-csrf", consoleChatBody)
			if w.Code != 401 || f.calls.Load() != 0 {
				t.Fatalf("invalid session reached inference: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestConsolePlaygroundEnforcesCurrentKeyPolicy(t *testing.T) {
	for _, scenario := range []string{"disabled", "deleted", "budget", "wallet", "model", "rpm", "tpm", "locked session"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConsoleFixture(t)
			want := 403
			body := consoleChatBody
			switch scenario {
			case "disabled":
				f.key.Status = "disabled"
			case "deleted":
				if err := f.repo.DeleteAPIKey(f.key.ID); err != nil {
					t.Fatal(err)
				}
			case "budget":
				f.key.Budget = 1
				f.key.UsedCost = 1
				want = 402
			case "wallet":
				if _, err := f.repo.GetDB().Exec(`UPDATE users SET balance=0, balance_units=0 WHERE id=?`, f.user.ID); err != nil {
					t.Fatal(err)
				}
				want = 402
			case "model":
				body = strings.Replace(consoleChatBody, "console-chat", "ungranted-model", 1)
			case "rpm":
				f.key.RPM = 1
				want = 429
			case "tpm":
				f.key.TPM = 1
				want = 429
			case "locked session":
				if _, err := f.repo.GetDB().Exec(`UPDATE users SET status='locked' WHERE id=?`, f.user.ID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "deleted" {
				if err := f.repo.UpdateAPIKey(f.key); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "budget" {
				if _, err := f.repo.GetDB().Exec(`UPDATE api_keys SET used_cost=1, used_cost_units=? WHERE id=?`, storage.ToUnits(1), f.key.ID); err != nil {
					t.Fatal(err)
				}
			}
			id := strconv.FormatInt(f.key.ID, 10)
			if scenario == "rpm" || scenario == "tpm" {
				w := f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", f.token, id, "console-csrf", body)
				if w.Code != 200 {
					t.Fatalf("initial rate-limit call: %d %s", w.Code, w.Body.String())
				}
			}
			before := f.calls.Load()
			w := f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", f.token, id, "console-csrf", body)
			if w.Code != want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
			}
			if f.calls.Load() != before {
				t.Fatal("rejected console call reached upstream")
			}
		})
	}
}

func TestConsolePlaygroundNativeAndFreeReadRoutes(t *testing.T) {
	f := newConsoleFixture(t)
	id := strconv.FormatInt(f.key.ID, 10)
	w := f.request("POST", consolePlaygroundPrefix+"/v1beta/models/console-gemini:generateContent", f.token, id, "console-csrf", `{"contents":[{"parts":[{"text":"hi"}]}]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "native console answer") {
		t.Fatalf("native route: %d %s", w.Code, w.Body.String())
	}
	// Empty wallet permits an owned, already-paid video task lookup. The handler
	// reports unknown task (404) rather than blocking admission with 402.
	if _, err := f.repo.GetDB().Exec(`UPDATE users SET balance=0, balance_units=0 WHERE id=?`, f.user.ID); err != nil {
		t.Fatal(err)
	}
	w = f.request("GET", consolePlaygroundPrefix+"/v1/videos/tasks/unknown", f.token, id, "", "")
	if w.Code != 404 {
		t.Fatalf("free video task read: %d %s", w.Code, w.Body.String())
	}
}

func TestConsolePlaygroundAdminAndSDKCredentials(t *testing.T) {
	f := newConsoleFixture(t)
	id := strconv.FormatInt(f.otherKey.ID, 10)
	w := f.request("POST", consolePlaygroundPrefix+"/v1/chat/completions", f.adminToken, id, "console-csrf", consoleChatBody)
	if w.Code != 200 {
		t.Fatalf("authorized admin selection: %d %s", w.Code, w.Body.String())
	}
	logs := edgeUsageRows(t, f.repo, 1)
	if logs[0].APIKey != f.otherKey.Key {
		t.Fatal("admin call billed a different key")
	}
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(consoleChatBody))
	req.Header.Set("Authorization", "Bearer "+f.secret)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("SDK credential regressed: %d %s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
}
