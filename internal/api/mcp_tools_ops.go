package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// executeOpsTools executes gateway cluster status, model routes, and usage log queries.
func (h *MCPHandler) executeOpsTools(ctx context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "airoute_cluster_status":
		// A16: channel inventory is gateway-internal. The transport already
		// rejects non-admin calls; this second check keeps in-process/stdio
		// invocations from bypassing that gate.
		if !h.callerIsAdmin(ctx, args) {
			return "Security violation: cluster status is restricted to gateway administrators", true, true
		}
		if h.repo != nil {
			channels, err := h.repo.ListChannels()
			if err == nil {
				type ChanStatus struct {
					Name          string `json:"name"`
					Type          string `json:"type"`
					Status        string `json:"status"`
					BreakerStatus string `json:"breaker_status"`
					Priority      int    `json:"priority"`
					Weight        int    `json:"weight"`
				}
				var statuses []ChanStatus
				activeCount := 0
				trippedCount := 0
				for _, c := range channels {
					if c.Status == "active" {
						activeCount++
					}
					if c.BreakerStatus == "open" {
						trippedCount++
					}
					statuses = append(statuses, ChanStatus{
						Name:          c.Name,
						Type:          string(c.Type),
						Status:        c.Status,
						BreakerStatus: c.BreakerStatus,
						Priority:      c.Priority,
						Weight:        c.Weight,
					})
				}
				clusterHealth := "OPERATIONAL"
				if trippedCount > 0 {
					clusterHealth = "DEGRADED"
				}
				res := gin.H{
					"cluster_health":   clusterHealth,
					"total_channels":   len(channels),
					"active_channels":  activeCount,
					"tripped_channels": trippedCount,
					"checked_at":       time.Now().Format("2006-01-02 15:04:05"),
					"channels":         statuses,
				}
				b, _ := json.MarshalIndent(res, "", "  ")
				return string(b), false, true
			}
		}
		return "Status OK", false, true

	case "airoute_model_topology":
		// A16: the model→provider topology is gateway-internal too.
		if !h.callerIsAdmin(ctx, args) {
			return "Security violation: model topology is restricted to gateway administrators", true, true
		}
		if h.dispatcher != nil {
			routes := h.dispatcher.GetModelRoutes()
			if len(routes) > 0 {
				type ModelItem struct {
					Model           string `json:"model"`
					Modality        string `json:"modality"`
					Providers       int    `json:"providers"`
					PrimaryCount    int    `json:"primary_count"`
					FallbackCount   int    `json:"fallback_count"`
					HasFallbackTier bool   `json:"has_fallback_tier"`
				}
				var list []ModelItem
				modalityFilter, _ := args["modality"].(string)
				for _, r := range routes {
					if modalityFilter != "" && r.Modality != modalityFilter {
						continue
					}
					list = append(list, ModelItem{
						Model:           r.Model,
						Modality:        r.Modality,
						Providers:       len(r.Providers),
						PrimaryCount:    r.PrimaryProvidersCount,
						FallbackCount:   r.FallbackProvidersCount,
						HasFallbackTier: r.HasFallbackTier,
					})
				}
				b, _ := json.MarshalIndent(list, "", "  ")
				return string(b), false, true
			}
		}
		return `[{"model":"deepseek-chat","modality":"chat","providers":2},{"model":"deepseek-reasoner","modality":"chat","providers":2},{"model":"gpt-4o","modality":"chat","providers":1}]`, false, true

	case "airoute_query_logs":
		if h.repo == nil {
			return "Storage repository not initialized", true, true
		}
		apiKey, _ := ctx.Value(contextKeyCallerKey).(string)
		// Context carries the stored digest; the legacy argument is cleartext.
		keyIsDigest := apiKey != ""
		if apiKey == "" {
			if k, ok := args["api_key"].(string); ok && k != "" {
				apiKey = k
			}
		}

		var callerKeyRec *storage.APIKeyRecord
		if apiKey != "" {
			lookupKey := apiKey
			if !keyIsDigest {
				lookupKey = storage.HashAPIKey(apiKey)
			}
			kRec, err := h.repo.GetAPIKeyByKey(lookupKey)
			if err == nil && kRec != nil {
				callerKeyRec = kRec
			}
		}

		sessionID, _ := args["session_id"].(string)

		// Tenant isolation & access control:
		if h.repo.CountAPIKeys() > 0 && callerKeyRec == nil {
			return "Error: Authentication required to query global logs. Please provide a valid API key.", true, true
		}

		limit := 10
		if lVal, ok := args["limit"].(float64); ok && lVal > 0 {
			limit = int(lVal)
		}
		filter := storage.LogFilter{
			Limit:     limit,
			SessionID: sessionID,
		}
		if callerKeyRec != nil {
			filter.APIKeys = []string{callerKeyRec.Key}
			filter.ScopeByAPIKeys = true
			filter.TenantID = callerKeyRec.TenantID
		}

		logs, err := h.repo.ListUsageLogsWithFilter(filter)
		if err != nil {
			return fmt.Sprintf("Query logs error: %v", err), true, true
		}
		// D-03: the audit rows carry the key digest (the ledger joins depend on
		// it), so the agent-facing projection must name keys the way the console
		// and the access log do (ak_…), masking a digest that no longer resolves.
		h.presentMCPLogKeys(logs)
		b, _ := json.MarshalIndent(logs, "", "  ")
		return string(b), false, true
	}

	return "", false, false
}
