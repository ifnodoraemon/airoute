package storage

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

func parseSQLTime(raw interface{}) *time.Time {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case time.Time:
		return &v
	case *time.Time:
		return v
	case string:
		if v == "" {
			return nil
		}
		layouts := []string{
			time.RFC3339,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, v); err == nil {
				return &t
			}
		}
	case []byte:
		return parseSQLTime(string(v))
	}
	return nil
}

func scanSupplierChannel(rows rowScanner) (*SupplierReconciliationItem, error) {
	var item SupplierReconciliationItem
	var firstRaw, lastRaw interface{}
	var avgDur, avgTtft, avgTps float64

	if err := rows.Scan(
		&item.ChannelName,
		&item.TotalRequests,
		&item.SuccessRequests,
		&item.FailedRequests,
		&item.TotalTokens,
		&item.PromptTokens,
		&item.UncachedPromptTokens,
		&item.CompletionTokens,
		&item.CachedTokens,
		&item.TotalCost,
		&avgDur,
		&avgTtft,
		&avgTps,
		&firstRaw,
		&lastRaw,
	); err != nil {
		return nil, fmt.Errorf("scan supplier reconciliation row error: %w", err)
	}

	item.AvgDurationMs = int64(math.Round(avgDur))
	item.AvgTTFTMs = int64(math.Round(avgTtft))
	item.AvgTPS = math.Round(avgTps*10) / 10
	if item.TotalRequests > 0 {
		item.SuccessRate = math.Round((float64(item.SuccessRequests)/float64(item.TotalRequests))*10000) / 100
	} else {
		item.SuccessRate = 100.0
	}

	item.FirstRequestAt = parseSQLTime(firstRaw)
	item.LastRequestAt = parseSQLTime(lastRaw)
	item.Models = make([]*SupplierModelReconciliation, 0)

	return &item, nil
}

func readSupplierChannels(rows *sql.Rows) ([]*SupplierReconciliationItem, map[string]*SupplierReconciliationItem, supplierReconciliationTotals, error) {
	supplierMap := make(map[string]*SupplierReconciliationItem)
	var ordered []*SupplierReconciliationItem
	var totals supplierReconciliationTotals
	for rows.Next() {
		item, err := scanSupplierChannel(rows)
		if err != nil {
			return nil, nil, totals, err
		}
		supplierMap[item.ChannelName] = item
		ordered = append(ordered, item)
		totals.add(item)
	}
	return ordered, supplierMap, totals, nil
}
