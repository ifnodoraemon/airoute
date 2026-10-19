package controlplane

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// buildRoutePricing resolves the price row to persist for the group being
// edited, WITHOUT writing it (the caller commits it with the rest of the route
// change).
//
// Two defects are fixed here:
//  1. It used to load the row with GetModelPrice(), which is hard-wired to the
//     "default" group. Editing a VIP/enterprise route therefore seeded the form
//     from (and wrote back) the default group's rates, silently dragging one
//     group's prices into another.
//  2. It wrote its own transaction, so a later channel failure left the price
//     applied.
func (h *AdminHandler) buildRoutePricing(req *UpdateModelRouteRequest) (*storage.ModelPriceRecord, error) {
	if req.PromptPrice == nil && req.CompletionPrice == nil && req.CacheReadPrice == nil && req.FixedPrice == nil &&
		req.OffPeakEnabled == nil && req.OffPeakMode == "" && req.OffPeakSlots == "" &&
		req.WeekendAllDay == nil && req.OffPeakStart == "" && req.OffPeakEnd == "" && req.OffPeakDiscount == nil {
		return nil, nil
	}
	currency := req.Currency
	if currency == "" {
		currency = "CNY"
	}
	groupName := strings.TrimSpace(req.GroupName)
	if groupName == "" {
		groupName = "default"
	}
	var pPrice, cPrice, cachePrice, fPrice float64
	// Read the exact group first; fall back to the default group only when this
	// group has no row yet (that mirrors how the billing engine resolves rates).
	existing, _ := h.repo.GetModelPriceWithGroup(req.Model, groupName)
	if existing != nil {
		// Start from the stored prices so a partial update (e.g. only the fixed
		// price) cannot silently reset prompt/completion prices to zero.
		pPrice = existing.PromptPrice
		cPrice = existing.CompletionPrice
		cachePrice = existing.CacheReadPrice
		fPrice = existing.FixedPrice
	}
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
	offEnabled := true
	offMode := "custom"
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
			return nil, err
		}
	}

	rec := &storage.ModelPriceRecord{
		Model:           req.Model,
		GroupName:       groupName,
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
	}
	return rec, nil
}
