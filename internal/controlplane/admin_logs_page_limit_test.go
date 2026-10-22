package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// The handler and the repository must agree on the page size. The handler used
// to accept limit=1000 while the repository silently clamped the result to 50
// rows, and the response echoed the requested value, so a client paginating by
// that number skipped rows it never received.
func TestListLogsEchoesTheEffectivePageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "page-limit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)

	rows := make([]*storage.UsageLogRecord, 0, storage.MaxUsageLogPageSize+10)
	for i := 0; i < storage.MaxUsageLogPageSize+10; i++ {
		rows = append(rows, &storage.UsageLogRecord{APIKey: "page-key", Model: "page-model", StatusCode: 200})
	}
	if err := repo.BatchRecordUsageLogs(rows); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandler{repo: repo}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?limit=1000", nil)
	h.ListLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Code  int                       `json:"code"`
		Data  []*storage.UsageLogRecord `json:"data"`
		Total int64                     `json:"total"`
		Limit int                       `json:"limit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Limit != storage.MaxUsageLogPageSize {
		t.Fatalf("echoed limit %d, want the clamped %d", body.Limit, storage.MaxUsageLogPageSize)
	}
	if len(body.Data) != storage.MaxUsageLogPageSize {
		t.Fatalf("page returned %d rows, want the clamped %d", len(body.Data), storage.MaxUsageLogPageSize)
	}
	if body.Total != storage.MaxUsageLogPageSize+10 {
		t.Fatalf("total %d, want %d", body.Total, storage.MaxUsageLogPageSize+10)
	}
}
