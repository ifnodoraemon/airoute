package controlplane

import "github.com/ifnodoraemon/airoute/internal/storage"

// ValidatePriceRecord checks a pricing rule before it is stored. Extracted from
// pricing_service.go to keep input validation separate from service orchestration.
func ValidatePriceRecord(rec *storage.ModelPriceRecord) error {
	if rec == nil {
		return nil
	}
	if err := storage.ValidatePriceCurrency(rec); err != nil {
		return &ValidationError{Message: err.Error()}
	}
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"提示价格", rec.PromptPrice},
		{"补全价格", rec.CompletionPrice},
		{"缓存读取价格", rec.CacheReadPrice},
		{"固定价格", rec.FixedPrice},
	} {
		if f.value < 0 {
			return &ValidationError{Message: f.name + "不能为负数"}
		}
	}
	return nil
}