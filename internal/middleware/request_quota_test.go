package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func requestQuotaFixture(t *testing.T, balance float64) (*storage.Repository, *storage.APIKeyRecord) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := storage.NewRepository(db)
	user := &storage.UserRecord{Username: "quota-user", PasswordHash: "synthetic", Role: "user", Status: "active", Balance: balance}
	if err := r.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &storage.APIKeyRecord{Key: "sk-quota-synthetic", UserID: user.ID, TenantID: user.Username, Status: "active"}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	return r, key
}

func TestRequestQuotaRejectsIncompleteTopUp(t *testing.T) {
	r, key := requestQuotaFixture(t, 1)
	quota, err := AcquireRequestQuota(context.Background(), r, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer quota.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(contextKeyRequestQuota, quota)
	if RequireRequestQuota(c, 50) || !c.IsAborted() || c.Writer.Status() != http.StatusPaymentRequired {
		t.Fatalf("unfunded top-up was admitted: status=%d", c.Writer.Status())
	}
}

func TestRequestQuotaPendingIsRetainedUntilSettlement(t *testing.T) {
	r, key := requestQuotaFixture(t, 0.05)
	quota, err := AcquireRequestQuota(context.Background(), r, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	rec := &storage.UsageLogRecord{APIKey: key.Key, TenantID: key.TenantID, Cost: 0.05, TotalTokens: 1, BillingID: "pending-request"}
	quota.Pend(rec)
	quota.Close()
	if _, err := AcquireRequestQuota(context.Background(), r, key.Key); !errors.Is(err, storage.ErrWalletReserved) {
		t.Fatalf("pending balance was reused: %v", err)
	}
	if err := r.SettleUsageCharge(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	quota.Close()
	if _, err := AcquireRequestQuota(context.Background(), r, key.Key); !errors.Is(err, storage.ErrWalletReserved) {
		t.Fatalf("old wallet balance was reused after commit: %v", err)
	}
}

func TestRequestQuotaDuplicateCloseCannotEraseAnotherRequest(t *testing.T) {
	r, key := requestQuotaFixture(t, 0.10)
	a, err := AcquireRequestQuota(context.Background(), r, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := AcquireRequestQuota(context.Background(), r, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	a.Close()
	a.Close()
	c, err := AcquireRequestQuota(context.Background(), r, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := AcquireRequestQuota(context.Background(), r, key.Key); !errors.Is(err, storage.ErrWalletReserved) {
		t.Fatalf("duplicate close erased another reservation: %v", err)
	}
}

func TestRequestQuotaStorageFailureFailsClosed(t *testing.T) {
	previous := storage.GetGlobalRepository()
	storage.SetGlobalRepository(nil)
	t.Cleanup(func() { storage.SetGlobalRepository(previous) })
	accounts := map[string]authAccount{
		"wallet":         {status: "active", role: "user", balance: 100, userID: 9},
		"ownerless-key":  {status: "active", budgeted: true},
		"budgeted-admin": {status: "active", role: "admin", budgeted: true},
	}
	for name, account := range accounts {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			_, ok := reserveAuthQuota(c, "synthetic", account, false)
			if ok || c.Writer.Status() != http.StatusServiceUnavailable || c.Writer.Header().Get("Retry-After") != "2" {
				t.Fatalf("billing failure admitted: status=%d headers=%v", c.Writer.Status(), c.Writer.Header())
			}
		})
	}
}
