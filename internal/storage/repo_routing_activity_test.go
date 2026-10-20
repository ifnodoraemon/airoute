package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRoutingActivityUsesWholeWindowAndRecordedTrails(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "routing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	now := time.Now()
	records := []*UsageLogRecord{
		{Model: "alias", Channel: "primary→backup", StatusCode: 200, CreatedAt: now, DurationMs: 100},
		{Model: "alias", Channel: "backup", StatusCode: 200, ErrorMessage: "stream failure", CreatedAt: now, DurationMs: 200},
		{Model: "alias", Channel: "old", StatusCode: 503, CreatedAt: now.Add(-48 * time.Hour)},
		{Model: "other", Channel: "primary→backup", StatusCode: 200, CreatedAt: now},
	}
	for i := 0; i < 60; i++ {
		records = append(records, &UsageLogRecord{Model: "alias", Channel: "primary", StatusCode: 200, CreatedAt: now})
	}
	for _, record := range records {
		if err := repo.RecordUsageLog(record); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetRoutingActivity("alias", now.Add(-24*time.Hour))
	if err != nil || got.Requests != 62 || got.Errors != 1 || got.Failovers != 1 || len(got.Recent) != 2 {
		t.Fatalf("wrong activity: %+v err=%v", got, err)
	}
	if got.Channels["backup"].Requests != 2 || got.Channels["backup"].DurationMs != 300 || got.Channels["primary"].Requests != 60 {
		t.Fatalf("wrong terminal channel attribution: %+v", got.Channels)
	}
}
