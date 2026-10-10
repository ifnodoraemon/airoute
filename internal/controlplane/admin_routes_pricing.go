package controlplane

import (
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// saveRoutePricing synchronizes route pricing rules and off-peak discount configurations.
func (h *AdminHandler) saveRoutePricing(req *UpdateModelRouteRequest) error {
	if req.PromptPrice == nil && req.CompletionPrice == nil && req.CacheReadPrice == nil && req.FixedPrice == nil {
		return nil
	}
	currency := req.Currency
	if currency == "" {
		currency = "CNY"
	}
	var pPrice, cPrice, cachePrice, fPrice float64
	if req.PromptPrice != nil {
		pPrice = *req.PromptPrice
	}
	if req.CompletionPrice != nil {
		cPrice = *req.CompletionPrice
	}
	if req.CacheReadPrice != nil {
		cachePrice = *req.CacheReadPrice
	}
	if req.FixedPrice != nil {
		fPrice = *req.FixedPrice
	}
	existing, _ := h.repo.GetModelPrice(req.Model)
	offEnabled := true
	offMode := "deepseek"
	offSlots := ""
	weekendAll := true
	offStart := "00:00"
	offEnd := "08:30"
	offDiscount := 0.5
	if existing != nil {
		offEnabled = existing.OffPeakEnabled
		offMode = existing.OffPeakMode
		offSlots = existing.OffPeakSlots
		weekendAll = existing.WeekendAllDay
		offStart = existing.OffPeakStart
		offEnd = existing.OffPeakEnd
		offDiscount = existing.OffPeakDiscount
	}
	if req.OffPeakEnabled != nil {
		offEnabled = *req.OffPeakEnabled
	}
	if req.OffPeakMode != "" {
		offMode = req.OffPeakMode
	}
	if req.OffPeakSlots != "" {
		offSlots = req.OffPeakSlots
	}
	if req.WeekendAllDay != nil {
		weekendAll = *req.WeekendAllDay
	}
	if req.OffPeakStart != "" {
		offStart = req.OffPeakStart
	}
	if req.OffPeakEnd != "" {
		offEnd = req.OffPeakEnd
	}
	if req.OffPeakDiscount != nil {
		offDiscount = *req.OffPeakDiscount
	}

	if offEnabled && offSlots != "" {
		if err := billing.ValidateSlotsOverlap(offSlots); err != nil {
			return err
		}
	}

	err := h.repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           req.Model,
		PromptPrice:     pPrice,
		CompletionPrice: cPrice,
		CacheReadPrice:  cachePrice,
		FixedPrice:      fPrice,
		Currency:        currency,
		OffPeakEnabled:  offEnabled,
		OffPeakMode:     offMode,
		OffPeakSlots:    offSlots,
		WeekendAllDay:   weekendAll,
		OffPeakStart:    offStart,
		OffPeakEnd:      offEnd,
		OffPeakDiscount: offDiscount,
	})
	if err == nil && billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}
	return err
}
