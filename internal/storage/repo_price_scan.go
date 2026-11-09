package storage

const modelPriceColumns = `id, model, COALESCE(group_name, 'default'), prompt_price, completion_price, cache_read_price, fixed_price, currency, COALESCE(off_peak_enabled, 1), COALESCE(off_peak_start, '00:00'), COALESCE(off_peak_end, '08:30'), COALESCE(off_peak_discount, 0.5), COALESCE(off_peak_mode, 'custom'), COALESCE(off_peak_slots, ''), COALESCE(weekend_all_day, 1), created_at, updated_at`

func scanModelPrice(row rowScanner) (*ModelPriceRecord, error) {
	var rec ModelPriceRecord
	var offEnabled, weekendAllDay int
	if err := row.Scan(&rec.ID, &rec.Model, &rec.GroupName, &rec.PromptPrice, &rec.CompletionPrice, &rec.CacheReadPrice, &rec.FixedPrice, &rec.Currency, &offEnabled, &rec.OffPeakStart, &rec.OffPeakEnd, &rec.OffPeakDiscount, &rec.OffPeakMode, &rec.OffPeakSlots, &weekendAllDay, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	rec.OffPeakEnabled = offEnabled == 1
	rec.WeekendAllDay = weekendAllDay == 1
	return &rec, nil
}
