package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func spillTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GATEWAY_USAGE_SPILL_DIR", dir)
	return dir
}

func TestUsageSpillRoundTripReplaysAndRemovesFile(t *testing.T) {
	dir := spillTestConfig(t)
	r, key, u := billingIDTestRepo(t)

	spill := newUsageSpill()
	if !spill.enabled() {
		t.Fatal("spill should be enabled with a writable directory")
	}
	records := []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-spill-1", "bill-spill-1", 1.00),
		billingRecord(key, u.Username, "tr-spill-2", "bill-spill-2", 0.50),
	}
	written, err := spill.append(records)
	if err != nil || written != 2 {
		t.Fatalf("append = %d, %v; want 2, nil", written, err)
	}
	if files := spillFiles(t, dir); len(files) != 1 {
		t.Fatalf("spill files = %v, want exactly one", files)
	}

	replayed, err := spill.replay(r)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed != 2 {
		t.Fatalf("replayed = %d, want 2", replayed)
	}
	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("usage_logs rows = %d, want 2", rows)
	}
	if files := spillFiles(t, dir); len(files) != 0 {
		t.Fatalf("replayed files were not removed: %v", files)
	}

	// The wallet must have moved exactly once.
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.50); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d", user.BalanceUnits, want)
	}
}

// Two replicas share the gateway-data volume. Replaying the same spilled records
// from a second file must not charge twice: billing_id makes it a no-op.
func TestUsageSpillReplayIsIdempotent(t *testing.T) {
	spillTestConfig(t)
	r, key, u := billingIDTestRepo(t)

	records := func() []*UsageLogRecord {
		return []*UsageLogRecord{
			billingRecord(key, u.Username, "tr-dup-spill", "bill-dup-spill", 2.00),
		}
	}
	for i := 0; i < 2; i++ {
		spill := newUsageSpill()
		if _, err := spill.append(records()); err != nil {
			t.Fatalf("append %d: %v", i+1, err)
		}
		if _, err := spill.replay(r); err != nil {
			t.Fatalf("replay %d: %v", i+1, err)
		}
	}

	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("usage_logs rows = %d, want 1 after replaying the same record twice", rows)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (charged once)", user.BalanceUnits, want)
	}
}

func TestUsageSpillCanBeDisabled(t *testing.T) {
	t.Setenv("GATEWAY_USAGE_SPILL_DIR", "-")
	spill := newUsageSpill()
	if spill.enabled() {
		t.Fatal("spill should be disabled by GATEWAY_USAGE_SPILL_DIR=-")
	}
	if _, err := spill.append([]*UsageLogRecord{{TraceID: "x"}}); err == nil {
		t.Fatal("append on a disabled spill must report an error")
	}
}

// A corrupt line must not prevent the healthy records around it from being
// replayed.
func TestUsageSpillSkipsCorruptLines(t *testing.T) {
	dir := spillTestConfig(t)
	r, key, u := billingIDTestRepo(t)

	spill := newUsageSpill()
	if _, err := spill.append([]*UsageLogRecord{billingRecord(key, u.Username, "tr-ok", "bill-ok", 1.00)}); err != nil {
		t.Fatal(err)
	}
	file := spillFiles(t, dir)[0]
	f, err := os.OpenFile(filepath.Join(dir, file), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json}\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	before := telemetry.GlobalMetrics.UsageLogSpillReplayFailures.Load()
	if _, err := spill.replay(r); err != nil {
		t.Fatalf("replay with a corrupt line: %v", err)
	}
	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("usage_logs rows = %d, want the one healthy record", rows)
	}
	if telemetry.GlobalMetrics.UsageLogSpillReplayFailures.Load() <= before {
		t.Fatal("corrupt spill line was not counted")
	}
}

func spillFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files
}

var _ = context.Background
