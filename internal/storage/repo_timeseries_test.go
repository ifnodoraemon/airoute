package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGetStatsTimeSeries(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "timeseries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	now := time.Now()
	// Insert 10 logs with known TTFTs and models
	for i := 1; i <= 10; i++ {
		record := &UsageLogRecord{
			APIKey:      "user-key",
			Model:       "gpt-4o",
			Channel:     "openai",
			StatusCode:  200,
			TotalTokens: 100,
			TTFTMs:      int64(i * 10), // 10, 20, ..., 100
			Cost:        0.01,
			CreatedAt:   now.Add(-time.Duration(i*2) * time.Minute),
		}
		if i == 1 {
			record.Model = "dall-e-3"
		} else if i == 2 {
			record.Model = "whisper-1"
		} else if i == 3 {
			record.StatusCode = 500
		}
		if err := repo.RecordUsageLog(record); err != nil {
			t.Fatal(err)
		}
	}

	// Insert another log for a different tenant
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey:      "other-key",
		Model:       "claude-3-opus",
		StatusCode:  200,
		TotalTokens: 500,
		TTFTMs:      200,
		CreatedAt:   now.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	// 1. Test 24h cluster-wide
	data24h, err := repo.GetStatsTimeSeries("24h", nil)
	if err != nil {
		t.Fatalf("GetStatsTimeSeries 24h failed: %v", err)
	}
	if len(data24h.Buckets) != 24 {
		t.Fatalf("expected 24 buckets, got %d", len(data24h.Buckets))
	}
	if data24h.Percentiles.Sampled != 11 {
		t.Fatalf("expected 11 samples, got %d", data24h.Percentiles.Sampled)
	}
	if data24h.Percentiles.P50 == nil || *data24h.Percentiles.P50 != 60 {
		t.Fatalf("expected P50=60, got %v", data24h.Percentiles.P50)
	}

	// 2. Test 1h cluster-wide
	data1h, err := repo.GetStatsTimeSeries("1h", nil)
	if err != nil {
		t.Fatalf("GetStatsTimeSeries 1h failed: %v", err)
	}
	if len(data1h.Buckets) != 12 {
		t.Fatalf("expected 12 buckets, got %d", len(data1h.Buckets))
	}

	// 3. Test scoped by API key
	dataScoped, err := repo.GetStatsTimeSeries("24h", []string{"user-key"})
	if err != nil {
		t.Fatalf("GetStatsTimeSeries scoped failed: %v", err)
	}
	if dataScoped.Percentiles.Sampled != 10 {
		t.Fatalf("expected 10 samples for scoped key, got %d", dataScoped.Percentiles.Sampled)
	}
	if dataScoped.ModalityStats["images"] != 1 {
		t.Fatalf("expected 1 image request, got %d", dataScoped.ModalityStats["images"])
	}
	if dataScoped.ModalityStats["audio"] != 1 {
		t.Fatalf("expected 1 audio request, got %d", dataScoped.ModalityStats["audio"])
	}
}
