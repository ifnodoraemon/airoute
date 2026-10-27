package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newWalletLedgerHandler(t *testing.T) (*AdminHandler, *storage.Repository, *storage.UserRecord, *storage.UserRecord) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	alice := &storage.UserRecord{Username: "alice", Email: "alice@example.com", PasswordHash: "x", Role: "user", Status: "active", Balance: 100, BalanceUnits: storage.ToUnits(100), GroupName: "default"}
	bob := &storage.UserRecord{Username: "bob", Email: "bob@example.com", PasswordHash: "x", Role: "user", Status: "active", Balance: 20, BalanceUnits: storage.ToUnits(20), GroupName: "default"}
	for _, user := range []*storage.UserRecord{alice, bob} {
		if err := repo.CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.CreateWalletLedger(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := repo.RecordWalletTransactionContext(ctx, &storage.WalletTransactionRecord{
		UserID: alice.ID, Type: storage.WalletTypeRecharge, DeltaUnits: storage.ToUnits(50),
		BalanceAfterUnits: storage.ToUnits(150), Operator: "stripe", Reason: "top up", RefID: "order:A1",
	}); err != nil {
		t.Fatal(err)
	}
	return NewAdminHandler(repo, nil, nil), repo, alice, bob
}

func invokeWalletLedger(t *testing.T, handler gin.HandlerFunc, target string, claims *AdminClaims) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	if claims != nil {
		c.Set("admin_claims", claims)
	}
	handler(c)
	return w
}

func TestWalletLedgerAdminListAndFilter(t *testing.T) {
	handler, _, alice, _ := newWalletLedgerHandler(t)

	all := invokeWalletLedger(t, handler.ListWalletTransactions, "/api/v1/admin/wallet/transactions", &AdminClaims{Username: "admin", Role: "admin"})
	if all.Code != http.StatusOK {
		t.Fatalf("all status=%d %s", all.Code, all.Body.String())
	}
	allEnvelope := decodeEnvelope(t, all)
	items, _ := allEnvelope["data"].([]interface{})
	if len(items) != 3 { // two openings + alice recharge
		t.Fatalf("all rows = %d: %s", len(items), all.Body.String())
	}

	filtered := invokeWalletLedger(t, handler.ListWalletTransactions,
		"/api/v1/admin/wallet/transactions?user_id="+int64String(alice.ID), &AdminClaims{Username: "admin", Role: "admin"})
	filteredEnvelope := decodeEnvelope(t, filtered)
	filteredItems, _ := filteredEnvelope["data"].([]interface{})
	if len(filteredItems) != 2 {
		t.Fatalf("filtered rows = %d", len(filteredItems))
	}
	first := filteredItems[0].(map[string]interface{})
	if first["username"] != "alice" || first["type"] != "recharge" || first["delta"] != float64(50) {
		t.Fatalf("first row = %+v", first)
	}
}

func TestWalletLedgerExportCSV(t *testing.T) {
	handler, _, _, _ := newWalletLedgerHandler(t)
	w := invokeWalletLedger(t, handler.ExportWalletTransactions,
		"/api/v1/admin/wallet/transactions/export", &AdminClaims{Username: "admin", Role: "admin"})
	if w.Code != http.StatusOK {
		t.Fatalf("export status=%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("content type = %q", w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	for _, want := range []string{"username", "alice", "recharge", "50.0000"} {
		if !strings.Contains(body, want) {
			t.Fatalf("csv lost %q:\n%s", want, body)
		}
	}
}

func TestUserWalletLedgerIsOwnerScoped(t *testing.T) {
	handler, _, alice, bob := newWalletLedgerHandler(t)

	aliceResp := invokeWalletLedger(t, handler.ListUserWalletTransactions, "/api/v1/user/wallet/transactions", &AdminClaims{Username: alice.Username, Role: "user"})
	if aliceResp.Code != http.StatusOK {
		t.Fatalf("alice status=%d %s", aliceResp.Code, aliceResp.Body.String())
	}
	envelope := decodeEnvelope(t, aliceResp)
	items, _ := envelope["data"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("alice rows = %d", len(items))
	}
	for _, item := range items {
		if item.(map[string]interface{})["user_id"] != float64(alice.ID) {
			t.Fatalf("leaked another user's row: %+v", item)
		}
	}

	bobResp := invokeWalletLedger(t, handler.ListUserWalletTransactions, "/api/v1/user/wallet/transactions", &AdminClaims{Username: bob.Username, Role: "user"})
	bobItems := decodeEnvelope(t, bobResp)["data"].([]interface{})
	if len(bobItems) != 1 {
		t.Fatalf("bob rows = %d", len(bobItems))
	}
}
