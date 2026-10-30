package api

// Regression coverage for the P2 MCP batch: A11 (SSE error Content-Type),
// A16 (infrastructure-disclosure tools are admin-only) and the A18 follow-up
// (MCP airoute_chat must namespace the session-affinity key like HTTP does).
//
// B7 (no usage-estimate fallback in airoute_chat) is already covered at HEAD by
// TestMCPChatRecordsPricedUsageAndSettles in mcp_authz_regression_test.go, which
// asserts estimated prompt/completion tokens and usage_source=estimated for an
// upstream that omits usage.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// seedMCPAdminKey creates an administrator account plus its API key.
func seedMCPAdminKey(t *testing.T, repo *storage.Repository, key string) *storage.APIKeyRecord {
	t.Helper()
	user := &storage.UserRecord{
		Username: "admin-" + key, Email: "admin-" + key + "@example.com",
		PasswordHash: "x", Role: "admin", Status: "active", Balance: 100,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	rec := &storage.APIKeyRecord{
		Key: key, TenantID: user.Username, UserID: user.ID, Status: "active", GroupName: "default",
	}
	if err := repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create admin key: %v", err)
	}
	return rec
}

// mcpToolsCallBody renders a JSON-RPC tools/call request.
func mcpToolsCallBody(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		t.Fatalf("marshal tools/call: %v", err)
	}
	return string(body)
}

// rpcErrorCode returns the JSON-RPC error code of a response body, or 0.
func rpcErrorCode(t *testing.T, body string) int {
	t.Helper()
	var resp struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode JSON-RPC response %q: %v", body, err)
	}
	if resp.Error == nil {
		return 0
	}
	return resp.Error.Code
}

// A11: an MCP SSE handshake rejected before the stream starts must not label its
// JSON error body as text/event-stream.
func TestFixMCPSSEUnauthorizedUsesJSONContentType(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	if err := repo.CreateAPIKey(&storage.APIKeyRecord{
		Key: "sk-existing", TenantID: "t", Status: "active", GroupName: "default",
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}
	h := NewMCPHandler(nil, repo)

	c, w := newTestContext(http.MethodGet, "/mcp/sse")
	h.HandleMCPSSE(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body: %s)", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("BUG: rejected SSE handshake carries Content-Type %q, want application/json", contentType)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Errorf("rejection body is not JSON (%q): %v", w.Body.String(), err)
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("rejection body has no error field: %s", w.Body.String())
	}
}

// A16: channel inventory and routing topology are gateway-internal. A tenant key
// must get a JSON-RPC permission error, an administrator must get the data.
func TestFixMCPInfraToolsRequireAdmin(t *testing.T) {
	repo := newMCPRegressionRepo(t)
	if err := repo.CreateChannel(&storage.ChannelRecord{
		Name: "secret-upstream", Type: model.ProviderOpenAI, BaseURL: "https://secret.invalid",
		APIKey: "none", Models: []string{"m"}, Protocols: []string{"openai_chat"}, Status: "active",
	}); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "secret-upstream", Type: model.ProviderOpenAI, BaseURL: "https://secret.invalid",
		Models: []string{"m"}, Status: "active", Weight: 1, Protocols: []string{"openai_chat"},
	}})
	h := NewMCPHandler(dispatcher, repo)
	// The client holds these credentials; the repository stores only their digests.
	const tenantCredential = "sk-tenant-infra"
	const adminCredential = "sk-admin-infra"
	seedMCPUserKey(t, repo, tenantCredential, "active", 10, nil, 100, 0)
	seedMCPAdminKey(t, repo, adminCredential)

	for _, tool := range []string{"airoute_cluster_status", "airoute_model_topology"} {
		t.Run(tool+"/tenant denied", func(t *testing.T) {
			resp := postMCPMessages(t, h, "/mcp/messages",
				mcpToolsCallBody(t, tool, map[string]any{}), "Bearer "+tenantCredential)
			if resp.Code != http.StatusOK {
				t.Fatalf("transport status = %d (body: %s)", resp.Code, resp.Body.String())
			}
			if code := rpcErrorCode(t, resp.Body.String()); code != -32002 {
				t.Errorf("BUG: tenant key got no permission error for %s (code=%d, body: %s)", tool, code, resp.Body.String())
			}
			if strings.Contains(resp.Body.String(), "secret-upstream") {
				t.Errorf("BUG: %s leaked channel inventory to a tenant: %s", tool, resp.Body.String())
			}
		})

		t.Run(tool+"/admin allowed", func(t *testing.T) {
			resp := postMCPMessages(t, h, "/mcp/messages",
				mcpToolsCallBody(t, tool, map[string]any{}), "Bearer "+adminCredential)
			if resp.Code != http.StatusOK {
				t.Fatalf("transport status = %d (body: %s)", resp.Code, resp.Body.String())
			}
			if code := rpcErrorCode(t, resp.Body.String()); code != 0 {
				t.Fatalf("admin call for %s failed with code %d: %s", tool, code, resp.Body.String())
			}
			// cluster_status exposes channel names; model_topology exposes the
			// model→provider route summary. Both must be non-empty for an admin.
			wantField := "secret-upstream"
			if tool == "airoute_model_topology" {
				wantField = "modality"
			}
			if !strings.Contains(resp.Body.String(), wantField) {
				t.Errorf("admin call for %s returned no infrastructure data: %s", tool, resp.Body.String())
			}
		})
	}

	// Open development mode (no key has ever been created) has no tenant boundary
	// to protect, so infrastructure tools stay available there; the transport
	// itself refuses anonymous callers as soon as a key exists.
	openHandler := NewMCPHandler(dispatcher, newMCPRegressionRepo(t))
	if out, isErr, _ := openHandler.executeOpsTools(context.Background(), "airoute_model_topology", map[string]any{}); isErr {
		t.Errorf("open mode denied an infrastructure tool: %q", out)
	}

	// Defence in depth: a direct in-process invocation (stdio adapter path) must
	// not skip the gate.
	ctx := context.WithValue(context.Background(), contextKeyCallerKey, storage.HashAPIKey(tenantCredential))
	out, isErr, handled := h.executeOpsTools(ctx, "airoute_cluster_status", map[string]any{})
	if !handled || !isErr || !strings.Contains(out, "administrators") {
		t.Errorf("direct tenant invocation was not denied: handled=%v isErr=%v out=%q", handled, isErr, out)
	}
	if strings.Contains(out, "secret-upstream") {
		t.Errorf("direct tenant invocation leaked channel inventory: %s", out)
	}
}

// A18 follow-up: MCP produces exactly the same scoped affinity key as HTTP for
// the same caller, so a client-supplied session id cannot cross tenant
// boundaries, and the router actually routes the two tenants apart.
func TestFixMCPChatSessionAffinityTenantScoped(t *testing.T) {
	// 1. HTTP and MCP must derive the identical namespace for the same caller.
	httpCtx, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	httpCtx.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{
		Key: "sk-tenant-a", TenantID: "tenant-a",
	})
	httpCtx.Request.Header.Set("X-Session-ID", "shared-session")
	httpKey := sessionAffinityKey(httpCtx, "shared-session")

	mcpKey := scopedSessionID(sessionIdentityFor("tenant-a", 0, "sk-tenant-a"), "shared-session")
	if httpKey != mcpKey {
		t.Errorf("HTTP and MCP namespace the same caller differently: http=%q mcp=%q", httpKey, mcpKey)
	}
	otherTenant := scopedSessionID(sessionIdentityFor("tenant-b", 0, "sk-tenant-b"), "shared-session")
	if otherTenant == httpKey {
		t.Errorf("BUG: two tenants with the same session id share the affinity key %q", httpKey)
	}
	if scopedSessionID(sessionIdentityFor("tenant-a", 0, "sk-tenant-a"), "shared-session") == "shared-session" {
		t.Error("BUG: the session affinity key is still the bare client value")
	}

	// 2. End-to-end: the two tenants must be able to land on different channels
	// for the same client session id.
	const modelName = "m"
	hitA, hitB := 0, 0
	newUpstream := func(counter *int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*counter++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		}))
	}
	srvA := newUpstream(&hitA)
	defer srvA.Close()
	srvB := newUpstream(&hitB)
	defer srvB.Close()

	repo := newMCPRegressionRepo(t)
	dispatcher := router.NewDispatcher([]model.ChannelConfig{
		{Name: "chan-a", Type: model.ProviderOpenAI, BaseURL: srvA.URL, Models: []string{modelName}, Protocols: []string{"openai_chat"}, Status: "active", Weight: 1, Priority: 1},
		{Name: "chan-b", Type: model.ProviderOpenAI, BaseURL: srvB.URL, Models: []string{modelName}, Protocols: []string{"openai_chat"}, Status: "active", Weight: 1, Priority: 1},
	})
	h := NewMCPHandler(dispatcher, repo)
	_, keyA := seedMCPUserKey(t, repo, "sk-aff-a", "active", 10, nil, 100, 0)
	_, keyB := seedMCPUserKey(t, repo, "sk-aff-b", "active", 10, nil, 100, 0)
	// seedMCPUserKey derives the tenant from the key name; make them distinct.
	if keyA.TenantID == keyB.TenantID {
		t.Fatalf("test fixture produced identical tenants %q", keyA.TenantID)
	}

	// Derive the expected channel through the real routing path instead of
	// re-implementing the selector's hash: the affinity strategy is an
	// implementation detail (it moved from modulo-N to rendezvous hashing), while
	// what this test pins is tenant isolation of the session key.
	routedChannel := func(scoped string) string {
		ctx := context.WithValue(context.Background(), router.ContextKeySessionID, scoped)
		candidates := dispatcher.GetChannelsForModelAndProtocolWithContext(ctx, modelName, "openai_chat")
		if len(candidates) != 2 {
			t.Fatalf("expected 2 candidate channels, got %d", len(candidates))
		}
		return candidates[0].Name
	}
	scopedA := func(sid string) string {
		return scopedSessionID(sessionIdentityFor(keyA.TenantID, keyA.UserID, keyA.Key), sid)
	}
	scopedB := func(sid string) string {
		return scopedSessionID(sessionIdentityFor(keyB.TenantID, keyB.UserID, keyB.Key), sid)
	}

	// Pick a session id the two namespaces route to different channels.
	sessionID := ""
	for i := 0; i < 500 && sessionID == ""; i++ {
		candidate := "sess-" + strings.Repeat("x", i%5) + "-" + strconv.Itoa(i)
		if routedChannel(scopedA(candidate)) != routedChannel(scopedB(candidate)) {
			sessionID = candidate
		}
	}
	if sessionID == "" {
		t.Fatal("could not find a session id that separates the two tenants")
	}

	call := func(keyDigest string) {
		// Context values carry the stored digest (the MCP handler stores caller.Key).
		ctx := context.WithValue(context.Background(), contextKeyCallerKey, keyDigest)
		out, isErr, handled := h.executeRoutingTools(ctx, "airoute_chat", map[string]any{
			"model": modelName, "message": "hi", "session_id": sessionID,
		})
		if !handled || isErr {
			t.Fatalf("airoute_chat failed for %s: handled=%v isErr=%v out=%q", keyDigest, handled, isErr, out)
		}
	}
	call(keyA.Key)
	call(keyB.Key)

	wantA := routedChannel(scopedA(sessionID))
	wantB := routedChannel(scopedB(sessionID))
	if wantA == wantB {
		t.Fatal("fixture error: tenants resolve to the same channel")
	}
	hits := map[string]int{"chan-a": hitA, "chan-b": hitB}
	// Distinct namespaces must spread the two tenants over both channels; a bare
	// session id (the HEAD behaviour) sends both to the same one.
	if hits[wantA] != 1 || hits[wantB] != 1 {
		t.Errorf("BUG: tenants with the same session id were not isolated (want one hit each on %s / %s, got a=%d b=%d)",
			wantA, wantB, hitA, hitB)
	}
}
