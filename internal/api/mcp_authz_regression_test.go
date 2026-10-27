package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression coverage for the MCP findings A02/A04/A08/A17/B3: the JSON-RPC
// transport used to authenticate the caller but never applied the data-plane
// admission rules, billed with a discarded pricing snapshot and let any caller
// push a response into another caller's SSE session.

func newMCPRegressionRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "mcp-authz.db"))
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return storage.NewRepository(db)
}

// newMCPChatUpstream builds a dispatcher whose channel serves every model in
// models, plus a counter of upstream calls. A rejected call must leave the
// counter at zero: that is the difference between "rejected" and "billed".
func newMCPChatUpstream(t *testing.T, models ...string) (*router.Dispatcher, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		// Deliberately omits the usage block: the audit row must fall back to the
		// shared estimator instead of recording zero tokens/cost.
		_, _ = io.WriteString(w, `{"id":"chatcmpl-mcp","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hello from upstream"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(srv.Close)
	return router.NewDispatcher([]model.ChannelConfig{{
		Name: "ch", Type: model.ProviderOpenAI, BaseURL: srv.URL,
		Models: models, Status: "active", Weight: 1,
	}}), &hits
}

func seedMCPUserKey(t *testing.T, repo *storage.Repository, key, userStatus string, balance float64, allowed []string, budget, usedCost float64) (*storage.UserRecord, *storage.APIKeyRecord) {
	t.Helper()
	username := "tenant-" + key
	user := &storage.UserRecord{
		Username: username, Email: username + "@example.com", PasswordHash: "x",
		Role: "user", Status: userStatus, Balance: balance,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	rec := &storage.APIKeyRecord{
		Key: key, TenantID: username, UserID: user.ID, Status: "active",
		AllowedModels: allowed, Budget: budget, UsedCost: usedCost, GroupName: "default",
	}
	if err := repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return user, rec
}

func postMCPMessages(t *testing.T, h *MCPHandler, target, body, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	c, w := newTestContext(http.MethodPost, target)
	c.Request.Body = io.NopCloser(strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		c.Request.Header.Set("Authorization", authHeader)
	}
	h.HandleMCPMessages(c)
	return w
}

func mcpChatBody(t *testing.T, modelName, message string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "airoute_chat",
			"arguments": map[string]any{
				"model":   modelName,
				"message": message,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

// A locked owner must be rejected by the MCP transport before any upstream call.
func TestMCPChatRejectsLockedAccount(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	dispatcher, hits := newMCPChatUpstream(t, "m")
	h := NewMCPHandler(dispatcher, repo)
	seedMCPUserKey(t, repo, "sk-mcp-locked", "locked", 10, nil, 100, 0)

	// The client presents the credential; api_keys.key stores only its digest.
	resp := postMCPMessages(t, h, "/mcp/messages", mcpChatBody(t, "m", "hi"), "Bearer sk-mcp-locked")

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body: %s)", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "account_locked") {
		t.Fatalf("body does not carry the account_locked code: %s", resp.Body.String())
	}
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("upstream called %d time(s) for a locked account, want 0", got)
	}
}

// An exhausted per-key budget must be rejected by the MCP transport.
func TestMCPChatRejectsKeyOverBudget(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	dispatcher, hits := newMCPChatUpstream(t, "m")
	h := NewMCPHandler(dispatcher, repo)
	seedMCPUserKey(t, repo, "sk-mcp-budget", "active", 10, nil, 1, 500)

	resp := postMCPMessages(t, h, "/mcp/messages", mcpChatBody(t, "m", "hi"), "Bearer sk-mcp-budget")

	if resp.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 (body: %s)", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "key_budget_exceeded") {
		t.Fatalf("body does not carry the key_budget_exceeded code: %s", resp.Body.String())
	}
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("upstream called %d time(s) for an over-budget key, want 0", got)
	}
}

// The key's model allowlist must gate airoute_chat even when called directly
// through the tool dispatcher (the stdio adapter path).
func TestMCPChatRejectsModelNotInAllowlist(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	dispatcher, hits := newMCPChatUpstream(t, "cheap-model", "expensive-model")
	h := NewMCPHandler(dispatcher, repo)
	_, keyRec := seedMCPUserKey(t, repo, "sk-mcp-allowlist", "active", 10, []string{"cheap-model"}, 100, 0)

	ctx := context.WithValue(context.Background(), contextKeyCallerKey, keyRec.Key)
	out, isErr, handled := h.executeRoutingTools(ctx, "airoute_chat", map[string]interface{}{
		"model":   "expensive-model",
		"message": "hi",
	})

	if !handled {
		t.Fatal("airoute_chat was not handled")
	}
	if !isErr {
		t.Fatalf("disallowed model was served: %q", out)
	}
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("upstream called %d time(s) for a disallowed model, want 0", got)
	}
}

// The MCP usage row must be priced from the shared estimator, carry the pricing
// snapshot (so settlement can validate the currency) and actually settle.
func TestMCPChatRecordsPricedUsageAndSettles(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	dispatcher, _ := newMCPChatUpstream(t, "m")
	h := NewMCPHandler(dispatcher, repo)

	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "m", GroupName: "default",
		PromptPrice: 1000, CompletionPrice: 4000, CacheReadPrice: 100, Currency: "CNY",
	}); err != nil {
		t.Fatalf("save model price: %v", err)
	}

	prevEngine, prevLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	billing.GlobalEngine = billing.NewEngine(repo)
	logger := storage.InitAsyncLogger(repo, 64, 8, 10*time.Millisecond)
	storage.InitBillingSettlement(repo)
	defer func() {
		storage.InitBillingSettlement(nil)
		logger.Stop()
		billing.GlobalEngine = prevEngine
		storage.GlobalAsyncLogger = prevLogger
	}()

	user, keyRec := seedMCPUserKey(t, repo, "sk-mcp-priced", "active", 10, nil, 100, 0)

	ctx := context.WithValue(context.Background(), contextKeyCallerKey, keyRec.Key)
	out, isErr, handled := h.executeRoutingTools(ctx, "airoute_chat", map[string]interface{}{
		"model":   "m",
		"message": strings.Repeat("a", 400),
	})
	if !handled || isErr {
		t.Fatalf("airoute_chat failed: handled=%v isErr=%v out=%q", handled, isErr, out)
	}

	logger.Stop()

	logs, err := repo.ListUsageLogs(10, 0)
	if err != nil || len(logs) == 0 {
		t.Fatalf("no usage row persisted: logs=%v err=%v", logs, err)
	}
	row := logs[0]
	if row.PromptTokens <= 0 || row.CompletionTokens <= 0 {
		t.Fatalf("usage row has no fallback estimate: %d/%d tokens", row.PromptTokens, row.CompletionTokens)
	}
	if row.Cost <= 0 {
		t.Fatalf("usage row cost = %v, want a positive charge", row.Cost)
	}
	if row.PricingSnapshot == nil {
		t.Fatal("usage row has no pricing snapshot; settlement cannot validate the currency")
	}
	if row.PricingSnapshot.Currency != "CNY" {
		t.Fatalf("pricing snapshot currency = %q, want CNY", row.PricingSnapshot.Currency)
	}
	if row.UsageSource != usageSourceEstimated {
		t.Fatalf("usage_source = %q, want %q", row.UsageSource, usageSourceEstimated)
	}
	if row.APIKey != keyRec.Key || row.TenantID != user.Username || row.Channel != "ch" {
		t.Fatalf("usage attribution = key %q tenant %q channel %q", row.APIKey, row.TenantID, row.Channel)
	}

	// The charge must have moved money, not just created an audit row.
	after, err := repo.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if after.Balance >= user.Balance {
		t.Fatalf("wallet balance %v did not decrease from %v: charge did not settle", after.Balance, user.Balance)
	}
	var settlements int
	if err := repo.GetDB().QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if settlements != 1 {
		t.Fatalf("settlements = %d, want 1", settlements)
	}
}

// A caller holding a valid key must not be able to push a response into another
// caller's SSE session.
func TestMCPMessagesRejectsCrossSessionInjection(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	h := NewMCPHandler(nil, repo)
	seedMCPUserKey(t, repo, "sk-mcp-victim", "active", 10, nil, 100, 0)
	seedMCPUserKey(t, repo, "sk-mcp-attacker", "active", 10, nil, 100, 0)

	sessionID := "mcp-0123456789abcdef0123456789abcdef"
	victim := &mcpSession{ch: make(chan []byte, 4), apiKey: storage.HashAPIKey("sk-mcp-victim")}
	h.sessions.Store(sessionID, victim)
	t.Cleanup(func() { h.sessions.Delete(sessionID) })

	resp := postMCPMessages(t, h, "/mcp/messages?sessionId="+sessionID,
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`, "Bearer sk-mcp-attacker")

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a foreign session (body: %s)", resp.Code, resp.Body.String())
	}
	if len(victim.ch) != 0 {
		t.Fatalf("response injected into another caller's session (%d queued message(s))", len(victim.ch))
	}
}

// An explicit key that does not own the session must not fall back to the
// session's key: it is a mismatch, not a missing credential.
func TestMCPMessagesRejectsMismatchedSessionKey(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	h := NewMCPHandler(nil, repo)
	seedMCPUserKey(t, repo, "sk-mcp-owner", "active", 10, nil, 100, 0)
	seedMCPUserKey(t, repo, "sk-mcp-other", "active", 10, nil, 100, 0)

	sessionID := "mcp-abcdef0123456789abcdef0123456789"
	sess := &mcpSession{ch: make(chan []byte, 4), apiKey: storage.HashAPIKey("sk-mcp-owner")}
	h.sessions.Store(sessionID, sess)
	t.Cleanup(func() { h.sessions.Delete(sessionID) })

	resp := postMCPMessages(t, h, "/mcp/messages?sessionId="+sessionID,
		`{"jsonrpc":"2.0","id":8,"method":"ping"}`, "Bearer sk-mcp-other")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 on a mismatched key (body: %s)", resp.Code, resp.Body.String())
	}
	if len(sess.ch) != 0 {
		t.Fatalf("mismatched caller pushed %d message(s) into the session", len(sess.ch))
	}

	// The session stays usable for its owner: binding must not break the
	// standard SSE transport for the legitimate client.
	resp = postMCPMessages(t, h, "/mcp/messages?sessionId="+sessionID,
		`{"jsonrpc":"2.0","id":9,"method":"ping"}`, "Bearer sk-mcp-owner")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the session owner (body: %s)", resp.Code, resp.Body.String())
	}
	if len(sess.ch) != 1 {
		t.Fatalf("owner's response was not pushed into the session (%d queued)", len(sess.ch))
	}
}

// Same-key traffic over an SSE session must keep working: this is the normal
// transport path every MCP client uses.
func TestMCPMessagesAcceptsMatchingSessionKey(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	h := NewMCPHandler(nil, repo)
	seedMCPUserKey(t, repo, "sk-mcp-normal", "active", 10, nil, 100, 0)

	sessionID := "mcp-00112233445566778899aabbccddeeff"
	sess := &mcpSession{ch: make(chan []byte, 4), apiKey: storage.HashAPIKey("sk-mcp-normal")}
	h.sessions.Store(sessionID, sess)
	t.Cleanup(func() { h.sessions.Delete(sessionID) })

	resp := postMCPMessages(t, h, "/mcp/messages?sessionId="+sessionID,
		`{"jsonrpc":"2.0","id":10,"method":"ping"}`, "Bearer sk-mcp-normal")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the session owner (body: %s)", resp.Code, resp.Body.String())
	}
	if len(sess.ch) != 1 {
		t.Fatalf("owner's response was not pushed into the session (%d queued)", len(sess.ch))
	}
}

// The session id is the only capability of the SSE transport when no key is
// supplied, so it must carry at least 128 bits of entropy.
func TestMCPGeneratedSessionIDsAreHighEntropy(t *testing.T) {
	id := genSessionID()
	raw := strings.TrimPrefix(id, "mcp-")
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("session id %q is not hex: %v", id, err)
	}
	if len(decoded) < 16 {
		t.Fatalf("session id carries %d bytes of entropy, want >= 16", len(decoded))
	}
}

// MCP requests must consume the same per-key RPM budget as /v1/* requests.
func TestMCPMessagesEnforcesRPM(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	h := NewMCPHandler(nil, repo)
	_, rec := seedMCPUserKey(t, repo, "sk-mcp-rpm", "active", 10, nil, 0, 0)
	rec.RPM = 1
	if err := repo.UpdateAPIKey(rec); err != nil {
		t.Fatalf("set rpm: %v", err)
	}

	first := postMCPMessages(t, h, "/mcp/messages", `{"jsonrpc":"2.0","id":11,"method":"ping"}`, "Bearer sk-mcp-rpm")
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200 (body: %s)", first.Code, first.Body.String())
	}
	second := postMCPMessages(t, h, "/mcp/messages", `{"jsonrpc":"2.0","id":12,"method":"ping"}`, "Bearer sk-mcp-rpm")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429 (body: %s)", second.Code, second.Body.String())
	}
}
