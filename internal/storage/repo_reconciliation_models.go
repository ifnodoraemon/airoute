package storage

import (
	"database/sql"
	"math"
)

func readSupplierModels(mRows *sql.Rows, supplierMap map[string]*SupplierReconciliationItem) {
	for mRows.Next() {
		channel, item := scanSupplierModel(mRows)
		if item != nil {
			if supplier, ok := supplierMap[channel]; ok {
				supplier.Models = append(supplier.Models, item)
			}
		}
	}
}

func scanSupplierModel(mRows rowScanner) (string, *SupplierModelReconciliation) {
	var chName string
	var mItem SupplierModelReconciliation
	var avgDur, avgTtft, mAvgTps float64
	err := mRows.Scan(
		&chName,
		&mItem.Model,
		&mItem.TotalRequests,
		&mItem.SuccessRequests,
		&mItem.FailedRequests,
		&mItem.TotalTokens,
		&mItem.PromptTokens,
		&mItem.UncachedPromptTokens,
		&mItem.CompletionTokens,
		&mItem.CachedTokens,
		&mItem.TotalCost,
		&avgDur,
		&avgTtft,
		&mAvgTps,
	)
	if err != nil {
		return "", nil
	}
	mItem.AvgDurationMs = int64(math.Round(avgDur))
	mItem.AvgTTFTMs = int64(math.Round(avgTtft))
	mItem.AvgTPS = math.Round(mAvgTps*10) / 10
	if mItem.TotalRequests > 0 {
		mItem.SuccessRate = math.Round((float64(mItem.SuccessRequests)/float64(mItem.TotalRequests))*10000) / 100
	} else {
		mItem.SuccessRate = 100.0
	}
	return chName, &mItem
}
