package storage

import (
	"fmt"
	"math"
	"time"
)

// ChannelReconciliationFilter defines parameters to filter supplier reconciliation data.
type ChannelReconciliationFilter struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Channel   string `json:"channel"`
	Model     string `json:"model"`
}

// SupplierModelReconciliation represents model-level aggregated metrics for a supplier.
type SupplierModelReconciliation struct {
	Model                string  `json:"model"`
	TotalRequests        int64   `json:"total_requests"`
	SuccessRequests      int64   `json:"success_requests"`
	FailedRequests       int64   `json:"failed_requests"`
	SuccessRate          float64 `json:"success_rate"`
	TotalTokens          int64   `json:"total_tokens"`
	PromptTokens         int64   `json:"prompt_tokens"`
	UncachedPromptTokens int64   `json:"uncached_prompt_tokens"`
	CompletionTokens     int64   `json:"completion_tokens"`
	CachedTokens         int64   `json:"cached_tokens"`
	AvgTPS               float64 `json:"avg_tps"`
	TotalCost            float64 `json:"total_cost"`
	AvgDurationMs        int64   `json:"avg_duration_ms"`
	AvgTTFTMs            int64   `json:"avg_ttft_ms"`
}

// SupplierReconciliationItem represents aggregated reconciliation stats for a single upstream supplier.
type SupplierReconciliationItem struct {
	ChannelID            int64                          `json:"channel_id"`
	ChannelName          string                         `json:"channel_name"`
	ChannelType          string                         `json:"channel_type"`
	ChannelStatus        string                         `json:"channel_status"`
	BaseURL              string                         `json:"base_url"`
	Priority             int                            `json:"priority"`
	Weight               int                            `json:"weight"`
	TotalRequests        int64                          `json:"total_requests"`
	SuccessRequests      int64                          `json:"success_requests"`
	FailedRequests       int64                          `json:"failed_requests"`
	SuccessRate          float64                        `json:"success_rate"`
	TotalTokens          int64                          `json:"total_tokens"`
	PromptTokens         int64                          `json:"prompt_tokens"`
	UncachedPromptTokens int64                          `json:"uncached_prompt_tokens"`
	CompletionTokens     int64                          `json:"completion_tokens"`
	CachedTokens         int64                          `json:"cached_tokens"`
	AvgTPS               float64                        `json:"avg_tps"`
	TotalCost            float64                        `json:"total_cost"`
	AvgDurationMs        int64                          `json:"avg_duration_ms"`
	AvgTTFTMs            int64                          `json:"avg_ttft_ms"`
	FirstRequestAt       *time.Time                     `json:"first_request_at,omitempty"`
	LastRequestAt        *time.Time                     `json:"last_request_at,omitempty"`
	Models               []*SupplierModelReconciliation `json:"models"`
}

// SupplierReconciliationSummary represents the top-level overview metrics across all suppliers.
type SupplierReconciliationSummary struct {
	TotalRequests        int64   `json:"total_requests"`
	SuccessRequests      int64   `json:"success_requests"`
	FailedRequests       int64   `json:"failed_requests"`
	SuccessRate          float64 `json:"success_rate"`
	TotalTokens          int64   `json:"total_tokens"`
	PromptTokens         int64   `json:"prompt_tokens"`
	UncachedPromptTokens int64   `json:"uncached_prompt_tokens"`
	CompletionTokens     int64   `json:"completion_tokens"`
	CachedTokens         int64   `json:"cached_tokens"`
	AvgTPS               float64 `json:"avg_tps"`
	TotalCost            float64 `json:"total_cost"`
	AvgDurationMs        int64   `json:"avg_duration_ms"`
	AvgTTFTMs            int64   `json:"avg_ttft_ms"`
	ActiveSuppliersCount int     `json:"active_suppliers_count"`
	TotalSuppliersCount  int     `json:"total_suppliers_count"`
	Currency             string  `json:"currency"`
}

// SupplierReconciliationReport is the complete reconciliation response.
type SupplierReconciliationReport struct {
	Summary   *SupplierReconciliationSummary `json:"summary"`
	Suppliers []*SupplierReconciliationItem  `json:"suppliers"`
	TimeRange struct {
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	} `json:"time_range"`
}

// GetSupplierReconciliation aggregates usage logs by supplier/channel with full token and cost reconciliation.
func (r *Repository) GetSupplierReconciliation(f ChannelReconciliationFilter) (*SupplierReconciliationReport, error) {
	var whereClause string
	var args []interface{}

	var startClause, endClause string
	if r.db.Dialect() == "postgres" {
		startClause = ` AND created_at >= ?`
		endClause = ` AND created_at <= ?`
	} else {
		startClause = ` AND datetime(created_at) >= datetime(?)`
		endClause = ` AND datetime(created_at) <= datetime(?)`
	}

	if f.StartTime != "" {
		whereClause += startClause
		args = append(args, f.StartTime)
	}
	if f.EndTime != "" {
		whereClause += endClause
		args = append(args, f.EndTime)
	}
	if f.Channel != "" {
		whereClause += ` AND (channel = ? OR channel LIKE ?)`
		args = append(args, f.Channel, "%"+f.Channel+"%")
	}
	if f.Model != "" {
		whereClause += ` AND model = ?`
		args = append(args, f.Model)
	}

	// 1. Fetch channel-level aggregates
	channelQuery := `SELECT 
		COALESCE(channel, '') as ch_name,
		COUNT(*) as total_reqs,
		SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) as succ_reqs,
		SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as fail_reqs,
		COALESCE(SUM(total_tokens), 0) as tot_tok,
		COALESCE(SUM(prompt_tokens), 0) as prm_tok,
		COALESCE(SUM(CASE WHEN prompt_tokens >= cached_tokens THEN prompt_tokens - cached_tokens ELSE prompt_tokens END), 0) as uncached_prm_tok,
		COALESCE(SUM(completion_tokens), 0) as cmp_tok,
		COALESCE(SUM(cached_tokens), 0) as cch_tok,
		COALESCE(SUM(cost), 0.0) as tot_cost,
		COALESCE(AVG(duration_ms), 0) as avg_dur,
		COALESCE(AVG(ttft_ms), 0) as avg_ttft,
		COALESCE(SUM(completion_tokens) * 1000.0 / NULLIF(SUM(CASE WHEN completion_tokens > 0 THEN (CASE WHEN duration_ms > ttft_ms AND ttft_ms > 0 THEN duration_ms - ttft_ms ELSE duration_ms END) ELSE 0 END), 0), 0) as avg_tps,
		MIN(created_at) as min_created,
		MAX(created_at) as max_created
	FROM usage_logs
	WHERE channel IS NOT NULL AND TRIM(channel) != '' ` + whereClause + `
	GROUP BY channel
	ORDER BY tot_cost DESC, total_reqs DESC`

	rows, err := r.db.Query(channelQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query supplier reconciliation error: %w", err)
	}
	defer rows.Close()

	supplierMap := make(map[string]*SupplierReconciliationItem)
	var orderedSuppliers []*SupplierReconciliationItem

	var summaryTotalReqs int64
	var summarySuccReqs int64
	var summaryFailReqs int64
	var summaryTotTok int64
	var summaryPrmTok int64
	var summaryUncachedPrmTok int64
	var summaryCmpTok int64
	var summaryCchTok int64
	var summaryTotCost float64
	var totalDurationWeighted int64
	var totalTtftWeighted int64
	var totalGenDurationMs float64

	for rows.Next() {
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

		supplierMap[item.ChannelName] = &item
		orderedSuppliers = append(orderedSuppliers, &item)

		summaryTotalReqs += item.TotalRequests
		summarySuccReqs += item.SuccessRequests
		summaryFailReqs += item.FailedRequests
		summaryTotTok += item.TotalTokens
		summaryPrmTok += item.PromptTokens
		summaryUncachedPrmTok += item.UncachedPromptTokens
		summaryCmpTok += item.CompletionTokens
		summaryCchTok += item.CachedTokens
		summaryTotCost += item.TotalCost
		totalDurationWeighted += item.AvgDurationMs * item.TotalRequests
		totalTtftWeighted += item.AvgTTFTMs * item.TotalRequests
		if item.AvgTPS > 0 {
			totalGenDurationMs += (float64(item.CompletionTokens) * 1000.0) / item.AvgTPS
		}
	}

	// 2. Fetch model-level breakdown per channel
	modelQuery := `SELECT 
		COALESCE(channel, '') as ch_name,
		COALESCE(model, 'unknown') as mdl_name,
		COUNT(*) as total_reqs,
		SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) as succ_reqs,
		SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as fail_reqs,
		COALESCE(SUM(total_tokens), 0) as tot_tok,
		COALESCE(SUM(prompt_tokens), 0) as prm_tok,
		COALESCE(SUM(CASE WHEN prompt_tokens >= cached_tokens THEN prompt_tokens - cached_tokens ELSE prompt_tokens END), 0) as uncached_prm_tok,
		COALESCE(SUM(completion_tokens), 0) as cmp_tok,
		COALESCE(SUM(cached_tokens), 0) as cch_tok,
		COALESCE(SUM(cost), 0.0) as tot_cost,
		COALESCE(AVG(duration_ms), 0) as avg_dur,
		COALESCE(AVG(ttft_ms), 0) as avg_ttft,
		COALESCE(SUM(completion_tokens) * 1000.0 / NULLIF(SUM(CASE WHEN completion_tokens > 0 THEN (CASE WHEN duration_ms > ttft_ms AND ttft_ms > 0 THEN duration_ms - ttft_ms ELSE duration_ms END) ELSE 0 END), 0), 0) as avg_tps
	FROM usage_logs
	WHERE channel IS NOT NULL AND TRIM(channel) != '' ` + whereClause + `
	GROUP BY channel, model
	ORDER BY channel ASC, tot_cost DESC, total_reqs DESC`

	mRows, err := r.db.Query(modelQuery, args...)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var chName string
			var mItem SupplierModelReconciliation
			var avgDur, avgTtft, mAvgTps float64
			if err := mRows.Scan(
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
			); err == nil {
				mItem.AvgDurationMs = int64(math.Round(avgDur))
				mItem.AvgTTFTMs = int64(math.Round(avgTtft))
				mItem.AvgTPS = math.Round(mAvgTps*10) / 10
				if mItem.TotalRequests > 0 {
					mItem.SuccessRate = math.Round((float64(mItem.SuccessRequests)/float64(mItem.TotalRequests))*10000) / 100
				} else {
					mItem.SuccessRate = 100.0
				}
				if sup, ok := supplierMap[chName]; ok {
					sup.Models = append(sup.Models, &mItem)
				}
			}
		}
	}

	// 3. Correlate with registered channels
	allChannels, _ := r.ListChannels()
	chByName := make(map[string]*ChannelRecord)
	for _, ch := range allChannels {
		chByName[ch.Name] = ch
	}

	for _, sup := range orderedSuppliers {
		if ch, ok := chByName[sup.ChannelName]; ok {
			sup.ChannelID = ch.ID
			sup.ChannelType = string(ch.Type)
			sup.ChannelStatus = ch.Status
			sup.BaseURL = ch.BaseURL
			sup.Priority = ch.Priority
			sup.Weight = ch.Weight
		} else {
			sup.ChannelStatus = "deleted"
			sup.ChannelType = "external"
		}
	}

	// If no channel filter was specified, also append registered channels with 0 usage
	if f.Channel == "" {
		for _, ch := range allChannels {
			if _, exists := supplierMap[ch.Name]; !exists {
				zeroItem := &SupplierReconciliationItem{
					ChannelID:     ch.ID,
					ChannelName:   ch.Name,
					ChannelType:   string(ch.Type),
					ChannelStatus: ch.Status,
					BaseURL:       ch.BaseURL,
					Priority:      ch.Priority,
					Weight:        ch.Weight,
					SuccessRate:   100.0,
					Models:        make([]*SupplierModelReconciliation, 0),
				}
				orderedSuppliers = append(orderedSuppliers, zeroItem)
			}
		}
	} else if len(orderedSuppliers) == 0 {
		for _, ch := range allChannels {
			if ch.Name == f.Channel {
				zeroItem := &SupplierReconciliationItem{
					ChannelID:     ch.ID,
					ChannelName:   ch.Name,
					ChannelType:   string(ch.Type),
					ChannelStatus: ch.Status,
					BaseURL:       ch.BaseURL,
					Priority:      ch.Priority,
					Weight:        ch.Weight,
					SuccessRate:   100.0,
					Models:        make([]*SupplierModelReconciliation, 0),
				}
				orderedSuppliers = append(orderedSuppliers, zeroItem)
				break
			}
		}
	}

	if orderedSuppliers == nil {
		orderedSuppliers = make([]*SupplierReconciliationItem, 0)
	}

	totalCount := len(allChannels)
	if len(orderedSuppliers) > totalCount {
		totalCount = len(orderedSuppliers)
	}

	// 4. Calculate overall summary
	summary := &SupplierReconciliationSummary{
		TotalRequests:        summaryTotalReqs,
		SuccessRequests:      summarySuccReqs,
		FailedRequests:       summaryFailReqs,
		TotalTokens:          summaryTotTok,
		PromptTokens:         summaryPrmTok,
		UncachedPromptTokens: summaryUncachedPrmTok,
		CompletionTokens:     summaryCmpTok,
		CachedTokens:         summaryCchTok,
		TotalCost:            math.Round(summaryTotCost*10000) / 10000,
		ActiveSuppliersCount: len(supplierMap),
		TotalSuppliersCount:  totalCount,
		Currency:             "CNY",
	}
	if summaryTotalReqs > 0 {
		summary.SuccessRate = math.Round((float64(summarySuccReqs)/float64(summaryTotalReqs))*10000) / 100
		summary.AvgDurationMs = totalDurationWeighted / summaryTotalReqs
		summary.AvgTTFTMs = totalTtftWeighted / summaryTotalReqs
		if totalGenDurationMs > 0 {
			summary.AvgTPS = math.Round((float64(summaryCmpTok)*1000.0/totalGenDurationMs)*10) / 10
		}
	} else {
		summary.SuccessRate = 100.0
	}

	report := &SupplierReconciliationReport{
		Summary:   summary,
		Suppliers: orderedSuppliers,
	}
	report.TimeRange.StartTime = f.StartTime
	report.TimeRange.EndTime = f.EndTime

	return report, nil
}

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
