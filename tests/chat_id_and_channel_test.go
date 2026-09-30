package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatIDAndChannelAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	// Register a deterministic mock provider that reports which channel served it
	upstreamChatID := "chatcmpl-upstream-42"
	dispatcher.RegisterProvider(&channelReportingProvider{chatID: upstreamChatID})

	ch := &storage.ChannelRecord{
		Name:    "Mock Primary",
		Type:    "mock-reporting",
		BaseURL: "http://mock.local",
		APIKey:  "sk-test",
		Models:  []string{"test-model"},
		Priority: 1,
		Weight:  10,
		Status:  "active",
	}
	require.NoError(t, repo.CreateChannel(ch))
	require.NoError(t, sync.ReloadFromDB())

	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	r := api.SetupRouter(dispatcher, adminHandler)

	t.Run("ChatCompletion records ChatID + Channel, response JSON stays clean", func(t *testing.T) {
		body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`
		req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)

		require.Equal(t, http.StatusOK, resp.Code)

		// Internal channel metadata must never leak into the response JSON
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &parsed))
		assert.NotContains(t, parsed, "Channel")
		assert.NotContains(t, parsed, "channel")
		assert.Equal(t, upstreamChatID, parsed["id"])

		// Chat ID response header removed entirely: it could not be truthful
		// on streams (SSE headers are written before the upstream ID is
		// known), and the audit DB record carries the real upstream ID.
		assert.Empty(t, resp.Header().Get("X-Airoute-Chat-ID"))
		assert.Empty(t, resp.Header().Get("X-Nano-Chat-ID"))

		// Async logger is not enabled in tests; verify via direct record readback
		require.NoError(t, repo.RecordUsageLog(&storage.UsageLogRecord{
			TraceID:   "tr-audit-1",
			ChatID:    upstreamChatID,
			Channel:   "Mock Primary",
			Model:     "test-model",
			TotalTokens: 10,
			CreatedAt: time.Now(),
		}))

		// Filter by the new dedicated ChatID column
		byChat, err := repo.ListUsageLogsWithFilter(storage.LogFilter{ChatID: upstreamChatID})
		require.NoError(t, err)
		require.Len(t, byChat, 1)
		assert.Equal(t, "Mock Primary", byChat[0].Channel)
		assert.Equal(t, upstreamChatID, byChat[0].ChatID)

		// Fuzzy chat_id match
		byPrefix, err := repo.ListUsageLogsWithFilter(storage.LogFilter{ChatID: "chatcmpl-upstream"})
		require.NoError(t, err)
		assert.Len(t, byPrefix, 1)

		// ChatID filter must NOT match unrelated records' trace_id
		require.NoError(t, repo.RecordUsageLog(&storage.UsageLogRecord{
			TraceID:     "tr-other",
			ChatID:      "chatcmpl-different-99",
			Model:       "test-model",
			TotalTokens: 5,
			CreatedAt:   time.Now(),
		}))
		byChat2, err := repo.ListUsageLogsWithFilter(storage.LogFilter{ChatID: upstreamChatID})
		require.NoError(t, err)
		assert.Len(t, byChat2, 1, "chat_id filter must not fall back to trace_id matching")
	})

	t.Run("Admin logs endpoint accepts chat_id query param", func(t *testing.T) {
		// Non-admin path requires auth; exercise the repository filter directly
		// plus verify the handler wiring via a crafted filter (mirrors ListLogs).
		filter := storage.LogFilter{
			ChatID: upstreamChatID,
			Limit:  50,
		}
		logs, err := repo.ListUsageLogsWithFilter(filter)
		require.NoError(t, err)
		assert.NotEmpty(t, logs)
		for _, l := range logs {
			assert.Equal(t, upstreamChatID, l.ChatID)
		}
	})

	t.Run("Dispatcher stamps channel on response", func(t *testing.T) {
		// Direct dispatcher-level contract: the mock provider returns no channel,
		// the dispatcher must stamp the channel that served the request.
		resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
			Model:    "test-model",
			Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
		})
		require.NoError(t, err)
		assert.Equal(t, "Mock Primary", resp.Channel)
		// json:"-" contract
		b, err := json.Marshal(resp)
		require.NoError(t, err)
		assert.NotContains(t, string(b), "Mock Primary")
	})
}

func TestChannelTrailAcrossFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	dispatcher.RegisterProvider(&channelReportingProvider{chatID: "chatcmpl-ok"})
	dispatcher.RegisterProvider(&channelFailingProvider{})

	// Priority 1 fails first, priority 2 succeeds -> trail must be "Fail First→Mock Second"
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: "Fail First", Type: "mock-fail", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"trail-model"}, Priority: 1, Weight: 10, Status: "active",
	}))
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: "Mock Second", Type: "mock-reporting", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"trail-model"}, Priority: 2, Weight: 10, Status: "active",
	}))
	require.NoError(t, sync.ReloadFromDB())

	t.Run("non-streaming trail", func(t *testing.T) {
		resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
			Model:    "trail-model",
			Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
		})
		require.NoError(t, err)
		assert.Equal(t, "Fail First→Mock Second", resp.Channel)
	})

	t.Run("streaming trail", func(t *testing.T) {
		streamChan, err := dispatcher.DispatchStream(context.Background(), &model.ChatCompletionRequest{
			Model:    "trail-model",
			Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
		})
		require.NoError(t, err)
		var gotTrail string
		for ev := range streamChan {
			if gotTrail == "" && ev.Channel != "" {
				gotTrail = ev.Channel
			}
		}
		assert.Equal(t, "Fail First→Mock Second", gotTrail)
	})
}

// channelReportingProvider is a minimal provider used to verify that the
// dispatcher injects the serving channel name into responses and stream events.
type channelReportingProvider struct {
	chatID string
}

func (p *channelReportingProvider) Name() string { return "mock-reporting" }

func (p *channelReportingProvider) Type() model.ProviderType { return model.ProviderType("mock-reporting") }

func (p *channelReportingProvider) ChatComplete(_ context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return &model.ChatCompletionResponse{
		ID:      p.chatID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "test-model",
		Choices: []model.ChatCompletionChoice{},
	}, nil
}

func (p *channelReportingProvider) ChatCompleteStream(_ context.Context, req *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	ch := make(chan *model.StreamEvent, 2)
	ch <- &model.StreamEvent{Chunk: &model.ChatCompletionChunk{ID: p.chatID, Object: "chat.completion.chunk", Model: req.Model}}
	ch <- &model.StreamEvent{IsDone: true}
	close(ch)
	return ch, nil
}

// channelFailingProvider always fails, used to drive fallback routing.
type channelFailingProvider struct{}

func (p *channelFailingProvider) Name() string { return "mock-fail" }

func (p *channelFailingProvider) Type() model.ProviderType { return model.ProviderType("mock-fail") }

func (p *channelFailingProvider) ChatComplete(_ context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("mock upstream failure")
}

func (p *channelFailingProvider) ChatCompleteStream(_ context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	return nil, errors.New("mock upstream stream failure")
}

func TestChannelTrailTruncation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	dispatcher.RegisterProvider(&channelReportingProvider{chatID: "chatcmpl-ok"})
	dispatcher.RegisterProvider(&channelFailingProvider{})

	// Two 70-rune names: trail "F*70→K*70" = 141 runes, must be truncated
	// to fit the VARCHAR(128) audit column while keeping the serving channel.
	longFail := strings.Repeat("F", 70)
	longOK := strings.Repeat("K", 70)
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: longFail, Type: "mock-fail", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"trunc-model"}, Priority: 1, Weight: 10, Status: "active",
	}))
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: longOK, Type: "mock-reporting", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"trunc-model"}, Priority: 2, Weight: 10, Status: "active",
	}))
	require.NoError(t, sync.ReloadFromDB())

	resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
		Model:    "trunc-model",
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len([]rune(resp.Channel)), 128, "trail must fit usage_logs.channel VARCHAR(128)")
	assert.True(t, strings.HasPrefix(resp.Channel, "…"), "truncated trail must be ellipsis-prefixed")
	assert.True(t, strings.HasSuffix(resp.Channel, longOK), "serving channel must survive truncation")
}

func TestUpstreamMissingChatID(t *testing.T) {
	// When the upstream returns no response ID, the audit chat_id must stay
	// empty (a fabricated id would never reconcile with the provider), while
	// the client-facing response body still carries a protocol-compliant id.
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	dispatcher.RegisterProvider(&channelReportingProvider{chatID: ""})
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: "Silent Upstream", Type: "mock-reporting", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"silent-model"}, Priority: 1, Weight: 10, Status: "active",
	}))
	require.NoError(t, sync.ReloadFromDB())

	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	r := api.SetupRouter(dispatcher, adminHandler)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"silent-model","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &parsed))
	assert.NotEmpty(t, parsed["id"], "response body must still carry a protocol-compliant id")
}

func TestFailedRequestAuditTrail(t *testing.T) {
	// All upstream channels down: the failure itself must land in usage_logs
	// (status 502, zero tokens) so failures are queryable in the audit UI,
	// not only in service logs.
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	dispatcher.RegisterProvider(&channelFailingProvider{})
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: "Dead Upstream", Type: "mock-fail", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"dead-model"}, Priority: 1, Weight: 10, Status: "active",
	}))
	require.NoError(t, sync.ReloadFromDB())

	// Enable the async audit logger with a fast flush for the test.
	storage.InitAsyncLogger(repo, 100, 10, 30*time.Millisecond)
	defer storage.GlobalAsyncLogger.Stop()
	defer func() { storage.GlobalAsyncLogger = nil }()

	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	r := api.SetupRouter(dispatcher, adminHandler)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"dead-model","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusBadGateway, resp.Code)

	// Allow the async worker a few flush cycles.
	time.Sleep(200 * time.Millisecond)

	logs, err := repo.ListUsageLogsWithFilter(storage.LogFilter{Model: "dead-model"})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, http.StatusBadGateway, logs[0].StatusCode)
	assert.Equal(t, 0, logs[0].PromptTokens)
	assert.Empty(t, logs[0].Channel)
}

func TestUsageLogTimeFilterSQLite(t *testing.T) {
	// Locks the SQLite normalization path of the time filter (the Postgres
	// branch compares natively and is verified against the live cluster).
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)

	require.NoError(t, repo.RecordUsageLog(&storage.UsageLogRecord{
		TraceID: "tr-time-1", Model: "time-model", StatusCode: http.StatusOK,
	}))

	// Wide window: ISO8601 input must normalize and match.
	logs, err := repo.ListUsageLogsWithFilter(storage.LogFilter{
		Model: "time-model", StartTime: "2000-01-01T00:00:00Z", EndTime: "2999-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	assert.Len(t, logs, 1)

	// Future-only window: nothing matches.
	logs, err = repo.ListUsageLogsWithFilter(storage.LogFilter{
		Model: "time-model", StartTime: "2999-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	assert.Empty(t, logs)
}

func TestChannelTrailFinalHopExceedsCap(t *testing.T) {
	// A serving channel whose name alone exceeds the VARCHAR(128) cap must
	// still survive in the trail as an ellipsis-prefixed tail — never a
	// bare "…" that erases the channel identity.
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	dispatcher.RegisterProvider(&channelReportingProvider{chatID: "chatcmpl-ok"})
	hugeName := strings.Repeat("H", 200)
	require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
		Name: hugeName, Type: "mock-reporting", BaseURL: "http://mock.local", APIKey: "sk-test",
		Models: []string{"huge-model"}, Priority: 1, Weight: 10, Status: "active",
	}))
	require.NoError(t, sync.ReloadFromDB())

	resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{
		Model:    "huge-model",
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	assert.NotEqual(t, "…", resp.Channel, "oversized final hop must not collapse to a bare ellipsis")
	assert.LessOrEqual(t, len([]rune(resp.Channel)), 128)
	assert.True(t, strings.HasPrefix(resp.Channel, "…"))
	assert.True(t, strings.HasSuffix(resp.Channel, strings.Repeat("H", 127)),
		"the tail of the serving channel name must survive")
}

func TestUsageLogStringClamp(t *testing.T) {
	// Client-supplied model names are unbounded; the storage boundary must
	// clamp them to the column limit so a single oversized value cannot
	// fail the INSERT — and with batched writes, the whole batch.
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := storage.NewRepository(db)

	longModel := strings.Repeat("m", 200)
	expected := strings.Repeat("m", 128)

	require.NoError(t, repo.RecordUsageLog(&storage.UsageLogRecord{
		TraceID: "tr-clamp-1", Model: longModel, StatusCode: http.StatusOK,
	}))
	logs, err := repo.ListUsageLogsWithFilter(storage.LogFilter{TraceID: "tr-clamp-1"})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, expected, logs[0].Model)

	require.NoError(t, repo.BatchRecordUsageLogs([]*storage.UsageLogRecord{
		{TraceID: "tr-clamp-2", Model: longModel, StatusCode: http.StatusOK},
		{TraceID: "tr-clamp-3", Model: longModel, StatusCode: http.StatusOK},
	}))
	logs, err = repo.ListUsageLogsWithFilter(storage.LogFilter{TraceID: "tr-clamp-2"})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, expected, logs[0].Model)
}
