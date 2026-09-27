package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceMiddleware_GenerationAndExtraction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.TraceMiddleware())

	var capturedTraceID string
	var capturedContextTraceID string

	r.GET("/test-trace", func(c *gin.Context) {
		capturedTraceID = middleware.GetTraceID(c)
		capturedContextTraceID = middleware.GetTraceIDFromContext(c.Request.Context())
		c.String(http.StatusOK, "ok")
	})

	// 1. Auto-generate when header is absent
	req1 := httptest.NewRequest("GET", "/test-trace", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.NotEmpty(t, capturedTraceID)
	assert.Equal(t, capturedTraceID, capturedContextTraceID)
	assert.Equal(t, capturedTraceID, w1.Header().Get(middleware.HeaderNanoTraceID))
	assert.Equal(t, capturedTraceID, w1.Header().Get(middleware.HeaderRequestID))

	// 2. Propagate existing X-Nano-Trace-ID
	customTraceID := "tr-custom-trace-999"
	req2 := httptest.NewRequest("GET", "/test-trace", nil)
	req2.Header.Set(middleware.HeaderNanoTraceID, customTraceID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, customTraceID, capturedTraceID)
	assert.Equal(t, customTraceID, w2.Header().Get(middleware.HeaderNanoTraceID))

	// 3. Propagate existing X-Request-ID
	reqID := "req-abc-xyz-123"
	req3 := httptest.NewRequest("GET", "/test-trace", nil)
	req3.Header.Set(middleware.HeaderRequestID, reqID)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, reqID, capturedTraceID)
}

func TestRepository_TraceIDPersistenceAndFilter(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)

	traceID1 := "tr-cluster-node1-1001"
	traceID2 := "tr-cluster-node2-2002"

	// Insert log 1
	err = repo.RecordUsageLog(&storage.UsageLogRecord{
		TraceID:          traceID1,
		ChatID:           "chatcmpl-node1-aaa",
		SessionID:        "sess-1",
		VirtualKey:       "sk-test-1",
		TenantID:         "tenant-a",
		Model:            "gpt-4o",
		Channel:          "openai-prod",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		Cost:             0.005,
		DurationMs:       250,
		StatusCode:       200,
		CreatedAt:        time.Now(),
	})
	require.NoError(t, err)

	// Insert log 2
	err = repo.RecordUsageLog(&storage.UsageLogRecord{
		TraceID:          traceID2,
		ChatID:           "chatcmpl-node2-bbb",
		SessionID:        "sess-2",
		VirtualKey:       "sk-test-2",
		TenantID:         "tenant-b",
		Model:            "claude-3-5-sonnet",
		Channel:          "anthropic-prod",
		PromptTokens:     200,
		CompletionTokens: 80,
		TotalTokens:      280,
		Cost:             0.012,
		DurationMs:       420,
		StatusCode:       200,
		CreatedAt:        time.Now(),
	})
	require.NoError(t, err)

	// 1. List all
	logs, err := repo.ListUsageLogs(10, 0)
	require.NoError(t, err)
	assert.Len(t, logs, 2)

	// 2. Filter by TraceID
	filteredLogs, err := repo.ListUsageLogsWithFilter(storage.LogFilter{
		TraceID: traceID1,
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, filteredLogs, 1)
	assert.Equal(t, traceID1, filteredLogs[0].TraceID)
	assert.Equal(t, "gpt-4o", filteredLogs[0].Model)

	// 3. Fallback auto trace_id if empty
	err = repo.RecordUsageLog(&storage.UsageLogRecord{
		ChatID:     "chatcmpl-no-trace",
		Model:      "deepseek-v3",
		DurationMs: 120,
		StatusCode: 200,
	})
	require.NoError(t, err)

	allLogs, err := repo.ListUsageLogs(10, 0)
	require.NoError(t, err)
	assert.Len(t, allLogs, 3)
	for _, l := range allLogs {
		assert.NotEmpty(t, l.TraceID, "TraceID should never be empty in persisted logs")
	}
}

func TestDB_DialectRebind(t *testing.T) {
	// SQLite instance
	sqliteDB, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer sqliteDB.Close()

	assert.Equal(t, "sqlite", sqliteDB.Dialect())

	// Test SQLite rebind leaves ? unchanged
	qSqlite := "SELECT id FROM usage_logs WHERE trace_id = ? AND status_code = ?"
	assert.Equal(t, qSqlite, sqliteDB.Rebind(qSqlite))

	// Test context trace extraction
	ctx := context.WithValue(context.Background(), middleware.ContextKeyTraceID, "tr-ctx-12345")
	assert.Equal(t, "tr-ctx-12345", middleware.GetTraceIDFromContext(ctx))
	assert.Empty(t, middleware.GetTraceIDFromContext(context.Background()))
}
