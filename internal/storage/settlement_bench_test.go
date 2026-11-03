package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// benchmarkSettlementRepo builds a file-backed SQLite repository (WAL, matching
// the single-node deployment mode) with one funded user and one API key.
func benchmarkSettlementRepo(b *testing.B) (*Repository, *APIKeyRecord) {
	b.Helper()
	db, err := OpenDB(filepath.Join(b.TempDir(), "settlement-bench.db"))
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })

	r := NewRepository(db)
	user := &UserRecord{
		Username:     "bench-user",
		PasswordHash: "synthetic",
		Balance:      1_000_000,
		BalanceUnits: ToUnits(1_000_000),
	}
	if err := r.CreateUser(user); err != nil {
		b.Fatalf("create user: %v", err)
	}
	key := &APIKeyRecord{Key: "sk-bench", UserID: user.ID, TenantID: user.Username}
	if err := r.CreateAPIKey(key); err != nil {
		b.Fatalf("create key: %v", err)
	}
	return r, key
}

// BenchmarkSettleChargesBatched measures the group-commit lever directly: K
// charges committed in one transaction versus one transaction per charge.
//
// This is the regression baseline for the settlement path. The audit that
// motivated batching measured ~2.8-3.5k charges/s at K=1 and ~20k at K=100 on
// SQLite WAL; a large regression here means the per-charge statement count or
// the commit path changed.
func BenchmarkSettleChargesBatched(b *testing.B) {
	r, key := benchmarkSettlementRepo(b)
	ctx := context.Background()

	for _, batchSize := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("K=%d", batchSize), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				batch := make([]*UsageLogRecord, batchSize)
				for j := range batch {
					batch[j] = billingRecord(key, key.TenantID, "tr-bench", NewBillingID(), 0.002)
				}
				if err := r.settleCharges(ctx, batch); err != nil {
					b.Fatalf("settle: %v", err)
				}
			}
			b.ReportMetric(float64(batchSize), "records/op")
		})
	}
}

// BenchmarkSettlementBatcherParallel exercises the request-facing API under
// concurrency, which is what production calls. It includes the queue, the
// flush, and the request-side wait.
func BenchmarkSettlementBatcherParallel(b *testing.B) {
	r, key := benchmarkSettlementRepo(b)
	previous := globalSettlementBatcher
	batcher := InitSettlementBatcher(r)
	globalSettlementBatcher = batcher
	b.Cleanup(func() {
		batcher.Stop()
		globalSettlementBatcher = previous
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rec := billingRecord(key, key.TenantID, "tr-bench-parallel", NewBillingID(), 0.002)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = SettleChargeBatched(ctx, rec)
			cancel()
		}
	})
}
