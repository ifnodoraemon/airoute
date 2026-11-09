package storage

import (
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
