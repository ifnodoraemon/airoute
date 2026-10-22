package storage

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

// The page size is a shared contract: the repository caps oversized requests,
// and the admin handler clamps to the same value so the limit it echoes is the
// one actually applied. Asking for 1000 rows must not return a silent 50.
func TestUsageLogPageLimitIsCapped(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "page-cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	rows := make([]*UsageLogRecord, 0, MaxUsageLogPageSize+50)
	for i := 0; i < MaxUsageLogPageSize+50; i++ {
		rows = append(rows, &UsageLogRecord{APIKey: "cap-key", Model: "cap-model", Channel: "cap-channel", StatusCode: 200})
	}
	if err := repo.BatchRecordUsageLogs(rows); err != nil {
		t.Fatal(err)
	}

	page, total, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(MaxUsageLogPageSize+50) {
		t.Fatalf("total=%d want=%d", total, MaxUsageLogPageSize+50)
	}
	if len(page) != MaxUsageLogPageSize {
		t.Fatalf("oversized limit must return exactly MaxUsageLogPageSize rows, got %d", len(page))
	}
}

// A keyword containing LIKE metacharacters must match literally: without
// escaping, "100%" behaved as a wildcard and matched unrelated rows.
func TestUsageLogKeywordMatchesWildcardsLiterally(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "like-escape.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	for _, model := range []string{"pct-100%", "pct-100x", "und_a_b", "und_axb"} {
		if err := repo.RecordUsageLog(&UsageLogRecord{APIKey: "kw-key", Model: model, StatusCode: 200}); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		keyword string
		want    string
	}{
		{"pct-100%", "pct-100%"},
		{"und_a_b", "und_a_b"},
	}
	for _, tc := range cases {
		page, total, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{Keyword: tc.keyword})
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(page) != 1 || page[0].Model != tc.want {
			t.Fatalf("keyword %q matched total=%d page=%d (first model=%q), want exactly %q", tc.keyword, total, len(page), page[0].Model, tc.want)
		}
	}

	// A plain substring still matches across both variants.
	_, total, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{Keyword: "pct-100"})
	if err != nil || total != 2 {
		t.Fatalf("plain keyword should match 2 rows, got %d (err=%v)", total, err)
	}
}

// The time range is applied on every page load. Wrapping the column in datetime()
// forced SQLite into a full scan; normalizing the parameter keeps the index usable
// while still honouring the RFC3339 values the console sends.
func TestUsageLogTimeFilterIsCorrectAndUsesIndex(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "time-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		record := &UsageLogRecord{APIKey: "time-key", Model: "time-model", StatusCode: 200, CreatedAt: base.Add(time.Duration(i) * time.Hour)}
		if err := repo.RecordUsageLog(record); err != nil {
			t.Fatal(err)
		}
	}

	filter := LogFilter{
		Limit:     50,
		StartTime: base.Add(1 * time.Hour).Format(time.RFC3339),
		EndTime:   base.Add(3 * time.Hour).Format(time.RFC3339),
	}
	page, total, err := repo.ListUsageLogsWithFilterAndCount(filter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page) != 3 {
		t.Fatalf("time range should be inclusive, got total=%d page=%d", total, len(page))
	}

	where, args := repo.buildUsageLogWhere(filter)
	rows, err := repo.db.Query(`EXPLAIN QUERY PLAN SELECT id FROM usage_logs WHERE 1=1`+where+` ORDER BY id DESC LIMIT 50`, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	if !strings.Contains(plan.String(), "idx_usage_created") {
		t.Fatalf("time filter no longer uses idx_usage_created:\n%s", plan.String())
	}
}

// Facets are cached per authorization scope. That removes two full-history
// DISTINCT scans from every console poll; clearing the trail must still empty
// the dropdowns immediately.
func TestUsageLogFacetsCacheIsScopedAndInvalidated(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "facets-cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	seed := func(tenant, model string) {
		t.Helper()
		if err := repo.RecordUsageLog(&UsageLogRecord{APIKey: tenant, TenantID: tenant, Model: model, Channel: tenant + "-channel", StatusCode: 200}); err != nil {
			t.Fatal(err)
		}
	}
	seed("tenant-a", "model-a1")
	seed("tenant-b", "model-b1")

	first, err := repo.ListUsageLogFacets(LogFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Models, []string{"model-a1"}) {
		t.Fatalf("unexpected first facets: %+v", first.Models)
	}

	// A new model appears, but the cached scope keeps serving the old options
	// until the TTL expires — the trade-off that removes the repeated scans.
	seed("tenant-a", "model-a2")
	cached, err := repo.ListUsageLogFacets(LogFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cached.Models, []string{"model-a1"}) {
		t.Fatalf("facets were not cached per scope: %+v", cached.Models)
	}

	// Another tenant must never observe this cache entry.
	other, err := repo.ListUsageLogFacets(LogFilter{TenantID: "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(other.Models, []string{"model-b1"}) {
		t.Fatalf("cache leaked across scopes: %+v", other.Models)
	}

	// Clearing the trail invalidates the cache immediately.
	if err := repo.ClearAllUsageLogs(); err != nil {
		t.Fatal(err)
	}
	cleared, err := repo.ListUsageLogFacets(LogFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Models) != 0 || len(cleared.Channels) != 0 {
		t.Fatalf("cleared trail still served cached facets: %+v", cleared)
	}
}

// Every console page filters by scope (tenant/key/model) plus a time range.
// Single-column indexes cannot serve that shape, so migration v18 adds
// composite ones; this pins both their existence and the scoped query they serve.
func TestUsageLogQueryIndexesExist(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "log-indexes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, name := range []string{"idx_usage_tenant_created", "idx_usage_key_created", "idx_usage_model_created"} {
		var found string
		if err := db.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&found); err != nil {
			t.Fatalf("index %s missing after migration: %v", name, err)
		}
	}

	repo := NewRepository(db)
	base := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		record := &UsageLogRecord{APIKey: "scope-key", TenantID: "tenant-a", Model: "scope-model", Channel: "scope-channel", StatusCode: 200, CreatedAt: base.Add(time.Duration(i) * time.Hour)}
		if err := repo.RecordUsageLog(record); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{
		Limit:     50,
		TenantID:  "tenant-a",
		StartTime: base.Add(1 * time.Hour).Format(time.RFC3339),
		EndTime:   base.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(page) != 2 {
		t.Fatalf("tenant+time scope broken: total=%d page=%d", total, len(page))
	}
}

// A flush can carry more rows than one multi-row INSERT chunk; the chunking must
// not drop rows, and rows without an explicit CreatedAt must still be stamped.
func TestBatchUsageLogInsertSpansChunks(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "chunked-insert.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	const total = 60 // > the 25-row insert chunk
	logs := make([]*UsageLogRecord, 0, total)
	wantTokens := int64(0)
	for i := 0; i < total; i++ {
		logs = append(logs, &UsageLogRecord{APIKey: "chunk-key", Model: "chunk-model", StatusCode: 200, TotalTokens: i})
		wantTokens += int64(i)
	}
	if err := repo.BatchRecordUsageLogs(logs); err != nil {
		t.Fatal(err)
	}

	page, count, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if count != total || len(page) != total {
		t.Fatalf("chunked insert lost rows: count=%d page=%d want=%d", count, len(page), total)
	}
	var gotTokens int64
	for _, l := range page {
		gotTokens += int64(l.TotalTokens)
		if l.CreatedAt.IsZero() {
			t.Fatalf("row %d lost its created_at stamp", l.ID)
		}
	}
	if gotTokens != wantTokens {
		t.Fatalf("token totals changed: got %d want %d", gotTokens, wantTokens)
	}
}
