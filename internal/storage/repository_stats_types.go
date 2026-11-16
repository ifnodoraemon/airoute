package storage

// StatsOverview aggregates system stats for the dashboard.
type StatsOverview struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalTokens       int64   `json:"total_tokens"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	TotalCachedTokens int64   `json:"total_cached_tokens"`
	TotalCost         float64 `json:"total_cost"`
	TodayCost         float64 `json:"today_cost"` // UTC calendar day
	MonthCost         float64 `json:"month_cost"` // UTC calendar month
	Cost24H           float64 `json:"cost_24h"`
	PreviousCost24H   float64 `json:"previous_cost_24h"`
	SavedCost         float64 `json:"saved_cost"`
	ActiveChannels    int     `json:"active_channels"`
	ActiveKeys        int     `json:"active_keys"`
	AvgTTFTMs         float64 `json:"avg_ttft_ms"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	CurrentRPM        int64   `json:"current_rpm"` // Real-time requests in the last 60 seconds
	CurrentTPM        int64   `json:"current_tpm"` // Real-time tokens in the last 60 seconds
	PeakRPM           int64   `json:"peak_rpm"`    // Peak RPM in recent timeframe
	PeakTPM           int64   `json:"peak_tpm"`    // Peak TPM in recent timeframe
	// Status breakdown across ALL usage logs (not just the page the console
	// happens to have loaded). The dashboard used to compute its SLA and error
	// figures from the 50-row log page, which made them meaningless.
	ClientErrorRequests int64 `json:"client_error_requests"` // 4xx
	ServerErrorRequests int64 `json:"server_error_requests"` // 5xx + other failures
	SuccessRequests     int64 `json:"success_requests"`
	// TTFT sample count, so the console can tell "no samples" apart from "0 ms".
	TTFTSamples int64 `json:"ttft_samples"`
}
