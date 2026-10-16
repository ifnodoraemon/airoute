package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestSupplierReconciliation(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	handler := NewAdminHandler(repo, nil, nil)

	// Create test channels
	ch1 := &storage.ChannelRecord{
		Name:    "SiliconFlow",
		Type:    model.ProviderOpenAI,
		BaseURL: "https://api.siliconflow.cn/v1",
		APIKey:  "sk-silicon-test",
		Status:  "active",
		Weight:  10,
	}
	if err := repo.CreateChannel(ch1); err != nil {
		t.Fatalf("create channel error: %v", err)
	}

	ch2 := &storage.ChannelRecord{
		Name:    "DeepSeek Direct",
		Type:    model.ProviderOpenAI,
		BaseURL: "https://api.deepseek.com",
		APIKey:  "sk-deepseek-test",
		Status:  "active",
		Weight:  20,
	}
	if err := repo.CreateChannel(ch2); err != nil {
		t.Fatalf("create channel error: %v", err)
	}

	// Insert test usage logs for reconciliation
	now := time.Now()
	testLogs := []*storage.UsageLogRecord{
		{
			TraceID:          "trace-1",
			ChatID:           "chatcmpl-sf-001",
			Model:            "deepseek-ai/DeepSeek-V3",
			Channel:          "SiliconFlow",
			PromptTokens:     100,
			CompletionTokens: 50,
			CachedTokens:     20,
			TotalTokens:      150,
			Cost:             0.005,
			StatusCode:       200,
			DurationMs:       250,
			TTFTMs:           120,
			CreatedAt:        now.Add(-1 * time.Hour),
		},
		{
			TraceID:          "trace-2",
			ChatID:           "chatcmpl-sf-002",
			Model:            "deepseek-ai/DeepSeek-V3",
			Channel:          "SiliconFlow",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     0,
			TotalTokens:      300,
			Cost:             0.010,
			StatusCode:       200,
			DurationMs:       350,
			TTFTMs:           180,
			CreatedAt:        now.Add(-30 * time.Minute),
		},
		{
			TraceID:          "trace-3",
			ChatID:           "chatcmpl-ds-001",
			Model:            "deepseek-reasoner",
			Channel:          "DeepSeek Direct",
			PromptTokens:     500,
			CompletionTokens: 400,
			CachedTokens:     100,
			TotalTokens:      900,
			Cost:             0.045,
			StatusCode:       200,
			DurationMs:       800,
			TTFTMs:           450,
			CreatedAt:        now.Add(-10 * time.Minute),
		},
		{
			TraceID:          "trace-4",
			ChatID:           "",
			Model:            "deepseek-reasoner",
			Channel:          "DeepSeek Direct",
			PromptTokens:     50,
			CompletionTokens: 0,
			CachedTokens:     0,
			TotalTokens:      50,
			Cost:             0.001,
			StatusCode:       502, // Error log
			DurationMs:       60,
			TTFTMs:           0,
			CreatedAt:        now.Add(-5 * time.Minute),
		},
		{
			TraceID:          "trace-empty-channel",
			ChatID:           "",
			Model:            "deepseek-reasoner",
			Channel:          "", // Early gateway failure without upstream channel
			PromptTokens:     0,
			CompletionTokens: 0,
			CachedTokens:     0,
			TotalTokens:      0,
			Cost:             0,
			StatusCode:       502,
			DurationMs:       10,
			TTFTMs:           0,
			CreatedAt:        now.Add(-2 * time.Minute),
		},
	}

	for _, l := range testLogs {
		if err := repo.RecordUsageLog(l); err != nil {
			t.Fatalf("record log error: %v", err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/admin/channels/reconciliation", nil)

	handler.GetSupplierReconciliation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int                                   `json:"code"`
		Data storage.SupplierReconciliationReport `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.Code != 0 {
		t.Fatalf("expected code 0, got %d", resp.Code)
	}

	// Verify no empty-name or phantom supplier exists
	for _, s := range resp.Data.Suppliers {
		if s.ChannelName == "" || s.ChannelType == "custom" {
			t.Fatalf("unexpected phantom supplier: %+v", s)
		}
	}

	summary := resp.Data.Summary
	if summary.TotalRequests != 4 {
		t.Fatalf("expected 4 total requests, got %d", summary.TotalRequests)
	}
	if summary.SuccessRequests != 3 {
		t.Fatalf("expected 3 success requests, got %d", summary.SuccessRequests)
	}
	if summary.FailedRequests != 1 {
		t.Fatalf("expected 1 failed request, got %d", summary.FailedRequests)
	}
	if summary.TotalTokens != 1400 {
		t.Fatalf("expected 1400 total tokens, got %d", summary.TotalTokens)
	}
	if summary.CachedTokens != 120 {
		t.Fatalf("expected 120 cached tokens, got %d", summary.CachedTokens)
	}
	if summary.UncachedPromptTokens != 730 {
		t.Fatalf("expected 730 uncached prompt tokens, got %d", summary.UncachedPromptTokens)
	}

	// Verify suppliers breakdown
	if len(resp.Data.Suppliers) < 2 {
		t.Fatalf("expected at least 2 suppliers, got %d", len(resp.Data.Suppliers))
	}

	var sfFound, dsFound bool
	for _, sup := range resp.Data.Suppliers {
		if sup.ChannelName == "SiliconFlow" {
			sfFound = true
			if sup.TotalRequests != 2 || sup.TotalTokens != 450 {
				t.Errorf("SiliconFlow expected 2 reqs, 450 tokens, got %d, %d", sup.TotalRequests, sup.TotalTokens)
			}
			if sup.UncachedPromptTokens != 280 {
				t.Errorf("SiliconFlow expected 280 uncached prompt tokens, got %d", sup.UncachedPromptTokens)
			}
			if sup.AvgTPS <= 0 {
				t.Errorf("SiliconFlow expected positive avg TPS, got %f", sup.AvgTPS)
			}
			if len(sup.Models) != 1 || sup.Models[0].Model != "deepseek-ai/DeepSeek-V3" {
				t.Errorf("SiliconFlow model breakdown incorrect: %+v", sup.Models)
			}
			if sup.Models[0].UncachedPromptTokens != 280 {
				t.Errorf("SiliconFlow model expected 280 uncached prompt tokens, got %d", sup.Models[0].UncachedPromptTokens)
			}
		}
		if sup.ChannelName == "DeepSeek Direct" {
			dsFound = true
			if sup.TotalRequests != 2 || sup.FailedRequests != 1 {
				t.Errorf("DeepSeek Direct expected 2 reqs, 1 fail, got %d, %d", sup.TotalRequests, sup.FailedRequests)
			}
			if sup.UncachedPromptTokens != 450 {
				t.Errorf("DeepSeek Direct expected 450 uncached prompt tokens, got %d", sup.UncachedPromptTokens)
			}
		}
	}

	if !sfFound || !dsFound {
		t.Fatalf("expected both SiliconFlow and DeepSeek Direct in reconciliation report")
	}
}
