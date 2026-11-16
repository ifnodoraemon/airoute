package controlplane

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// buildRoutePricing merges a route editor's partial price update onto the stored
// rate. Extracted from route_service.go to keep the route orchestration file
// under the size red line.
func (s *RouteService) buildRoutePricing(req *UpdateModelRouteRequest) (*storage.ModelPriceRecord, error) {
	if req.PromptPrice == nil && req.CompletionPrice == nil && req.CacheReadPrice == nil && req.FixedPrice == nil &&
		req.OffPeakEnabled == nil && req.OffPeakMode == "" && req.OffPeakSlots == "" &&
		req.WeekendAllDay == nil && req.OffPeakStart == "" && req.OffPeakEnd == "" && req.OffPeakDiscount == nil && req.Currency == "" {
		return nil, nil
	}
	currency := req.Currency
	groupName := strings.TrimSpace(req.GroupName)
	if groupName == "" {
		groupName = "default"
	}
	var pPrice, cPrice, cachePrice, fPrice float64
	// Read the exact group first; fall back to the default group only when this
	// group has no row yet (that mirrors how the billing engine resolves rates).
	existing, err := s.store.GetModelPriceWithGroup(req.Model, groupName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, &RouteStageError{Stage: RouteStageRead, Err: err}
	}
	if currency == "" && existing != nil {
		currency = existing.Currency
	}
	if currency == "" {
		currency = "CNY"
	}
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
		// Same rule as the pricing screen: the slot list is only read by the
		// engine in OffPeakMode=="custom", so that is the only mode whose slots
		// are validated here (billing.ValidateOffPeakSlots).
		if err := billing.ValidateOffPeakSlots(offMode, offSlots); err != nil {
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
	// The route editor is the second writer of a price row, so it applies the same
	// price validation the pricing screen does.
	if err := ValidatePriceRecord(rec); err != nil {
		return nil, err
	}
	return rec, nil
}