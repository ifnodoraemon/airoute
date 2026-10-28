package api

import (
	"context"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newMCPTestRepo(t *testing.T) (*MCPHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewMCPHandler(nil, repo), repo
}

// TestMCPAuthOpenOnlyWithoutAnyKeys pins the gate: anonymous access is allowed
// only before the first key is created. A deployment whose keys are all
// disabled must not silently fall back to open mode.
func TestMCPAuthOpenOnlyWithoutAnyKeys(t *testing.T) {
	h, repo := newMCPTestRepo(t)

	rec, err := h.authenticateCaller("")
	if err != nil || rec != nil {
		t.Fatalf("expected open mode with no keys, got rec=%v err=%v", rec, err)
	}

	disabled := &storage.APIKeyRecord{Key: "sk-disabled-key", TenantID: "t", Status: "disabled", GroupName: "default"}
	if err := repo.CreateAPIKey(disabled); err != nil {
		t.Fatalf("create disabled key: %v", err)
	}

	if _, err := h.authenticateCaller(""); err == nil {
		t.Fatal("expected an auth error when every key is disabled")
	}
	if _, err := h.authenticateCaller("sk-disabled-key"); err == nil {
		t.Fatal("expected a disabled key to be rejected")
	}

	active := &storage.APIKeyRecord{Key: "sk-active-key", TenantID: "t", Status: "active", GroupName: "default"}
	if err := repo.CreateAPIKey(active); err != nil {
		t.Fatalf("create active key: %v", err)
	}
	if _, err := h.authenticateCaller(""); err == nil {
		t.Fatal("expected an auth error for an anonymous caller once a key exists")
	}
	if rec, err := h.authenticateCaller("sk-active-key"); err != nil || rec == nil {
		t.Fatalf("expected the active key to authenticate, got rec=%v err=%v", rec, err)
	}
}

// TestMCPCallerIsAdminGatesSQLTools verifies that the SQL reflection tools are
// only available to an API key owned by a gateway administrator.
func TestMCPCallerIsAdminGatesSQLTools(t *testing.T) {
	h, repo := newMCPTestRepo(t)

	admin := &storage.UserRecord{Username: "admin", Email: "admin@example.com", PasswordHash: "x", Role: "admin", Status: "active"}
	if err := repo.CreateUser(admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	user := &storage.UserRecord{Username: "alice", Email: "alice@example.com", PasswordHash: "x", Role: "user", Status: "active"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	adminKey := &storage.APIKeyRecord{Key: "sk-admin-key", TenantID: "admin", UserID: admin.ID, Status: "active", GroupName: "default"}
	userKey := &storage.APIKeyRecord{Key: "sk-user-key", TenantID: "alice", UserID: user.ID, Status: "active", GroupName: "default"}
	if err := repo.CreateAPIKey(adminKey); err != nil {
		t.Fatalf("create admin key: %v", err)
	}
	if err := repo.CreateAPIKey(userKey); err != nil {
		t.Fatalf("create user key: %v", err)
	}

	withKey := func(key string) context.Context {
		return context.WithValue(context.Background(), contextKeyCallerKey, key)
	}

	// Context values carry the stored digest (the MCP handler stores caller.Key).
	if !h.callerIsAdmin(withKey(adminKey.Key), nil) {
		t.Fatal("expected the admin-owned key to be treated as administrator")
	}
	if h.callerIsAdmin(withKey(userKey.Key), nil) {
		t.Fatal("expected a regular user's key to be denied SQL tools")
	}
	if h.callerIsAdmin(context.Background(), nil) {
		t.Fatal("expected an anonymous context to be denied SQL tools")
	}
	// The stdio adapter has no HTTP request to carry a credential, so it passes
	// api_key as a tool argument. One shared predicate now means every admin-gated
	// tool accepts that path — before this the infrastructure tools did and the
	// SQL tools did not.
	if !h.callerIsAdmin(context.Background(), map[string]interface{}{"api_key": adminKey.Key}) {
		t.Fatal("expected the api_key argument to grant administrator authority")
	}
	if h.callerIsAdmin(context.Background(), map[string]interface{}{"api_key": userKey.Key}) {
		t.Fatal("expected a regular user's api_key argument to be denied")
	}

	// The tool itself must refuse for a non-admin even with a valid key.
	if out, isErr, _ := h.executeBuiltinUtilityTools(withKey("sk-user-key"), "read_query", map[string]interface{}{"query": "SELECT * FROM usage_logs"}); !isErr {
		t.Fatalf("expected read_query to be denied for a non-admin, got %q", out)
	}
	if out, isErr, _ := h.executeBuiltinUtilityTools(withKey("sk-user-key"), "list_tables", map[string]interface{}{}); !isErr {
		t.Fatalf("expected list_tables to be denied for a non-admin, got %q", out)
	}
	if out, isErr, _ := h.executeBuiltinUtilityTools(withKey("sk-user-key"), "describe_table", map[string]interface{}{"table_name": "channels"}); !isErr {
		t.Fatalf("expected describe_table to be denied for a non-admin, got %q", out)
	}
}
