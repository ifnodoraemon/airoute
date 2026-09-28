package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestTimeOfUseBilling(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tou_test.db")

	db, err := storage.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	engine := billing.NewEngine(repo)

	// Model 1: Custom Multi-Slot Mode (Workday 00:00-09:00, 12:00-14:00, 18:00-24:00 + weekends 50% discount)
	priceRec := &storage.ModelPriceRecord{
		Model:           "deepseek-chat",
		PromptPrice:     2.0,  // ¥2.00 / 1M
		CompletionPrice: 8.0,  // ¥8.00 / 1M
		CacheReadPrice:  0.2,  // ¥0.20 / 1M
		FixedPrice:      0.0,
		Currency:        "CNY",
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakDiscount: 0.5,
		OffPeakSlots:    `[{"start":"00:00","end":"09:00","discount":0.5},{"start":"12:00","end":"14:00","discount":0.5},{"start":"18:00","end":"24:00","discount":0.5}]`,
		WeekendAllDay:   true,
	}
	if err := repo.SaveModelPrice(priceRec); err != nil {
		t.Fatalf("SaveModelPrice failed: %v", err)
	}

	// Model 2: Custom Multi-Slot Mode (00:00-08:30 50% off, 12:00-14:00 70% off)
	priceCustom := &storage.ModelPriceRecord{
		Model:           "custom-model",
		PromptPrice:     10.0,
		CompletionPrice: 20.0,
		CacheReadPrice:  1.0,
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakSlots:    `[{"start":"00:00","end":"08:30","discount":0.5},{"start":"12:00","end":"14:00","discount":0.7}]`,
		WeekendAllDay:   true,
		OffPeakDiscount: 0.5,
	}
	if err := repo.SaveModelPrice(priceCustom); err != nil {
		t.Fatalf("SaveModelPrice custom failed: %v", err)
	}

	if err := engine.ReloadPrices(); err != nil {
		t.Fatalf("ReloadPrices failed: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")

	// --- Case 1: DeepSeek Official Workday Peak (Wednesday 2026-09-23 14:30) ---
	workdayPeak := time.Date(2026, 9, 23, 14, 30, 0, 0, loc)
	p := engine.GetPrice("deepseek-chat")
	isOff, discount := engine.IsOffPeak(p, workdayPeak)
	if isOff {
		t.Errorf("Expected peak at Wednesday 14:30, got off-peak")
	}
	if discount != 1.0 {
		t.Errorf("Expected discount 1.0 at peak, got %v", discount)
	}
	cost, totalSaved, _, offPeak, disc := engine.CalculateCostDetailed("deepseek-chat", 1_000_000, 1_000_000, 0, workdayPeak)
	if offPeak || disc != 1.0 || cost != 10.0 || totalSaved != 0.0 {
		t.Errorf("DeepSeek peak calculation mismatch: cost=%v, disc=%v", cost, disc)
	}

	// --- Case 2: DeepSeek Official Workday Lunch Off-Peak (Wednesday 2026-09-23 12:30) ---
	workdayLunch := time.Date(2026, 9, 23, 12, 30, 0, 0, loc)
	isOffLunch, discountLunch := engine.IsOffPeak(p, workdayLunch)
	if !isOffLunch || discountLunch != 0.5 {
		t.Errorf("Expected DeepSeek lunch off-peak (0.5), got %v (isOff=%v)", discountLunch, isOffLunch)
	}

	// --- Case 3: DeepSeek Official Workday Night Off-Peak (Wednesday 2026-09-23 03:15) ---
	workdayNight := time.Date(2026, 9, 23, 3, 15, 0, 0, loc)
	isOffNight, discountNight := engine.IsOffPeak(p, workdayNight)
	if !isOffNight || discountNight != 0.5 {
		t.Errorf("Expected DeepSeek night off-peak (0.5), got %v (isOff=%v)", discountNight, isOffNight)
	}

	// --- Case 4: DeepSeek Official Weekend Full Day Off-Peak (Saturday 2026-09-26 14:30) ---
	weekendAfternoon := time.Date(2026, 9, 26, 14, 30, 0, 0, loc)
	isOffWeekend, discountWeekend := engine.IsOffPeak(p, weekendAfternoon)
	if !isOffWeekend || discountWeekend != 0.5 {
		t.Errorf("Expected DeepSeek weekend all-day off-peak (0.5), got %v", discountWeekend)
	}

	// --- Case 5: Custom Multi-Slot Evaluation ---
	pCust := engine.GetPrice("custom-model")
	// Slot 1: 03:00 (inside 00:00-08:30) -> 50% discount (0.5)
	isOffC1, discC1 := engine.IsOffPeak(pCust, time.Date(2026, 9, 23, 3, 0, 0, 0, loc))
	if !isOffC1 || discC1 != 0.5 {
		t.Errorf("Custom slot 1 failed: isOff=%v, disc=%v", isOffC1, discC1)
	}
	// Slot 2: 13:00 (inside 12:00-14:00) -> 70% discount (0.7)
	isOffC2, discC2 := engine.IsOffPeak(pCust, time.Date(2026, 9, 23, 13, 0, 0, 0, loc))
	if !isOffC2 || discC2 != 0.7 {
		t.Errorf("Custom slot 2 failed: isOff=%v, disc=%v", isOffC2, discC2)
	}
	// Outside slots: 10:00 -> Peak (1.0)
	isOffC3, discC3 := engine.IsOffPeak(pCust, time.Date(2026, 9, 23, 10, 0, 0, 0, loc))
	if isOffC3 || discC3 != 1.0 {
		t.Errorf("Custom outside slot failed: isOff=%v, disc=%v", isOffC3, discC3)
	}

	// --- Case 6: Off-Peak Hour + Prompt Caching Stacking (Double Discount!) ---
	// 1,000,000 prompt tokens with 800,000 cached tokens + 200,000 completion tokens
	// Uncached prompt: 200k tokens = 0.2 * ¥2.00 = ¥0.40
	// Cached prompt: 800k tokens = 0.8 * ¥0.20 = ¥0.16
	// Completion: 200k tokens = 0.2 * ¥8.00 = ¥1.60
	// Subtotal before off-peak discount = ¥0.40 + ¥0.16 + ¥1.60 = ¥2.16
	// Subtotal with 50% off-peak discount = ¥2.16 * 0.5 = ¥1.08!
	cost3, totalSaved3, cacheSaved3, _, _ := engine.CalculateCostDetailed("deepseek-chat", 1_000_000, 200_000, 800_000, workdayNight)
	expectedBase := (0.2 * 2.0) + (0.8 * 0.2) + (0.2 * 8.0)
	expectedDiscounted := expectedBase * 0.5
	if cost3 < expectedDiscounted-0.0001 || cost3 > expectedDiscounted+0.0001 {
		t.Errorf("Expected stacked cost ~%v, got %v", expectedDiscounted, cost3)
	}
	if cacheSaved3 != 0.8*(2.0-0.2) {
		t.Errorf("Expected cacheSaved=%v, got %v", 0.8*1.8, cacheSaved3)
	}
	if totalSaved3 <= cacheSaved3 {
		t.Errorf("Expected totalSaved (%v) to be strictly greater than cacheSaved (%v)", totalSaved3, cacheSaved3)
	}
}
