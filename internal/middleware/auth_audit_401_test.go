package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestAuditAuthFailureSkipsInbound401 pins the recording policy: a caller that
// never authenticated (401) must not create a usage-log row, because no key
// matched and no request reached routing. A real admission denial on an
// authenticated key (402) must still be recorded.
func TestAuditAuthFailureSkipsInbound401(t *testing.T) {
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "auth401.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	previousLogger := storage.GlobalAsyncLogger
	previousRepo := storage.GetGlobalRepository()
	repo := storage.NewRepository(db)
	logger := storage.InitAsyncLogger(repo, 8, 8, time.Hour)
	defer func() {
		logger.Stop()
		storage.GlobalAsyncLogger = previousLogger
		storage.SetGlobalRepository(previousRepo)
	}()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	c.Set("trace_id", "tr-missing-key")
	auditAuthFailure(c, "", http.StatusUnauthorized, "Missing authentication key")

	c.Set("trace_id", "tr-invalid-key")
	auditAuthFailure(c, "sk-not-a-real-key", http.StatusUnauthorized, "Incorrect or unauthorized API key provided")

	c.Set("trace_id", "tr-insufficient-balance")
	auditAuthFailure(c, "sk-broke", http.StatusPaymentRequired, "Insufficient balance")

	// Flush deterministically before reading the table.
	logger.Stop()

	var unauthorized, denial int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE status_code = 401`).Scan(&unauthorized); err != nil {
		t.Fatal(err)
	}
	if unauthorized != 0 {
		t.Fatalf("inbound 401 rows = %d, want 0", unauthorized)
	}

	var source string
	row := db.QueryRow(`SELECT usage_source FROM usage_logs WHERE status_code = 402 AND trace_id = 'tr-insufficient-balance'`)
	if err := row.Scan(&source); err != nil {
		t.Fatalf("402 denial was not recorded: %v", err)
	}
	if source != "gateway_auth" {
		t.Fatalf("denial usage_source = %q, want gateway_auth", source)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&denial); err != nil {
		t.Fatal(err)
	}
	if denial != 1 {
		t.Fatalf("usage_logs rows = %d, want exactly the one denial", denial)
	}
}
