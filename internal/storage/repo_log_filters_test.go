package storage

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLogFiltersFacetsAndOverviewAgree(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "logs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	for i := 0; i < 60; i++ {
		record := &UsageLogRecord{APIKey: "own-key", Model: "new-model", Channel: "new-channel", StatusCode: 200, DurationMs: 800}
		if i == 0 {
			record.Model, record.Channel, record.StatusCode = "historic-model", "deleted-channel", 503
		}
		if i == 1 {
			record.ErrorMessage = "stream failed after headers"
		}
		if i == 2 {
			record.StatusCode = 429
		}
		if err := repo.RecordUsageLog(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.RecordUsageLog(&UsageLogRecord{APIKey: "other-key", Model: "private-model", Channel: "private-channel", StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	page, total, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{Limit: 50, Status: "error"})
	if err != nil || total != 3 || len(page) != 3 {
		t.Fatalf("errors page=%d total=%d err=%v", len(page), total, err)
	}
	stats, err := repo.GetStatsOverview()
	if err != nil || stats.ClientErrorRequests+stats.ServerErrorRequests != total {
		t.Fatalf("overview mismatch: %+v err=%v", stats, err)
	}
	owned, err := repo.GetStatsOverviewForKeys([]string{"own-key"})
	if err != nil || owned.ClientErrorRequests+owned.ServerErrorRequests != total {
		t.Fatalf("scoped overview mismatch: %+v err=%v", owned, err)
	}
	_, fast, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{Latency: "fast", Status: "error"})
	if err != nil || fast != 3 {
		t.Fatalf("800ms errors hidden: %d %v", fast, err)
	}
	facets, err := repo.ListUsageLogFacets(LogFilter{APIKeys: []string{"own-key"}, ScopeByAPIKeys: true, Channel: "new-channel", Status: "success", Offset: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(facets.Channels, []string{"deleted-channel", "new-channel"}) || !reflect.DeepEqual(facets.Models, []string{"historic-model", "new-model"}) {
		t.Fatalf("facets lost historical values or leaked other account: %+v", facets)
	}
	empty, err := repo.ListUsageLogFacets(LogFilter{ScopeByAPIKeys: true})
	if err != nil || len(empty.Channels) != 0 || len(empty.Models) != 0 {
		t.Fatalf("empty scope leaked facets: %+v %v", empty, err)
	}
}
