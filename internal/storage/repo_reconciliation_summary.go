package storage

import "math"

type supplierReconciliationTotals struct {
	SupplierReconciliationSummary
	durationWeighted     int64
	ttftWeighted         int64
	generationDurationMs float64
}

func (totals *supplierReconciliationTotals) add(item *SupplierReconciliationItem) {
	totals.TotalRequests += item.TotalRequests
	totals.SuccessRequests += item.SuccessRequests
	totals.FailedRequests += item.FailedRequests
	totals.TotalTokens += item.TotalTokens
	totals.PromptTokens += item.PromptTokens
	totals.UncachedPromptTokens += item.UncachedPromptTokens
	totals.CompletionTokens += item.CompletionTokens
	totals.CachedTokens += item.CachedTokens
	totals.TotalCost += item.TotalCost
	totals.durationWeighted += item.AvgDurationMs * item.TotalRequests
	totals.ttftWeighted += item.AvgTTFTMs * item.TotalRequests
	if item.AvgTPS > 0 {
		totals.generationDurationMs += (float64(item.CompletionTokens) * 1000.0) / item.AvgTPS
	}
}

func supplierReconciliationSummary(totals supplierReconciliationTotals, activeCount, totalCount int) *SupplierReconciliationSummary {
	// 4. Calculate overall summary
	summary := &SupplierReconciliationSummary{
		TotalRequests:        totals.TotalRequests,
		SuccessRequests:      totals.SuccessRequests,
		FailedRequests:       totals.FailedRequests,
		TotalTokens:          totals.TotalTokens,
		PromptTokens:         totals.PromptTokens,
		UncachedPromptTokens: totals.UncachedPromptTokens,
		CompletionTokens:     totals.CompletionTokens,
		CachedTokens:         totals.CachedTokens,
		TotalCost:            math.Round(totals.TotalCost*10000) / 10000,
		ActiveSuppliersCount: activeCount,
		TotalSuppliersCount:  totalCount,
		Currency:             "CNY",
	}
	if totals.TotalRequests > 0 {
		summary.SuccessRate = math.Round((float64(totals.SuccessRequests)/float64(totals.TotalRequests))*10000) / 100
		summary.AvgDurationMs = totals.durationWeighted / totals.TotalRequests
		summary.AvgTTFTMs = totals.ttftWeighted / totals.TotalRequests
		if totals.generationDurationMs > 0 {
			summary.AvgTPS = math.Round((float64(totals.CompletionTokens)*1000.0/totals.generationDurationMs)*10) / 10
		}
	} else {
		summary.SuccessRate = 100.0
	}

	return summary
}
