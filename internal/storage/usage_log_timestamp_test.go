package storage

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Every persisted row must keep sub-second precision. Whole-second values made
// a flush of 502s indistinguishable (43 rows all at 02:09:44), so retry spacing,
// concurrency and cross-system correlation could not be reconstructed.
func TestUsageLogTimestampsKeepMicroseconds(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	exact := time.Date(2026, 10, 23, 2, 9, 44, 905597000, time.UTC)
	rec := billingRecord(key, u.Username, "tr-precise", "bill-precise", 0.01)
	rec.CreatedAt = exact

	// Batch path (>1 record) and single path must both preserve the value.
	batch := []*UsageLogRecord{
		rec,
		billingRecord(key, u.Username, "tr-precise-2", "bill-precise-2", 0.01),
	}
	if err := r.BatchRecordUsageLogsContext(ctx, batch); err != nil {
		t.Fatalf("batch write: %v", err)
	}
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-precise-3", "bill-precise-3", 0.01),
	}); err != nil {
		t.Fatalf("single write: %v", err)
	}

	var stored string
	if err := r.db.QueryRow(`SELECT created_at FROM usage_logs WHERE trace_id = 'tr-precise'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, ".905597") {
		t.Fatalf("created_at = %q, want the microsecond fraction preserved", stored)
	}

	// The zero-CreatedAt path used to fall back to the column default, which is
	// whole-second on SQLite; it must also carry a fraction now.
	var auto string
	if err := r.db.QueryRow(`SELECT created_at FROM usage_logs WHERE trace_id = 'tr-precise-3'`).Scan(&auto); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(auto, ".") {
		t.Fatalf("created_at = %q for a zero CreatedAt row, want sub-second precision", auto)
	}

	// Two rows of one batch must remain orderable: distinct timestamps for rows
	// that were recorded at different moments.
	var distinct int
	if err := r.db.QueryRow(`SELECT COUNT(DISTINCT created_at) FROM usage_logs WHERE trace_id IN ('tr-precise','tr-precise-2')`).Scan(&distinct); err != nil {
		t.Fatal(err)
	}
	if distinct != 2 {
		t.Fatalf("rows in one flush share a timestamp (distinct=%d): the flush collapsed them to the same instant", distinct)
	}
}
