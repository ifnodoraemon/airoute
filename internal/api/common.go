package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// AuditRecordParams encapsulates parameters required to record usage, calculate billing,
// and update system telemetry. Applying Separation of Concerns (SoC) and DRY.
type AuditRecordParams struct {
	SessionID        string
	ChatID           string
	Channel          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	TotalTokens      int
	Duration         time.Duration
	TTFT             time.Duration
	StatusCode       int
}

// RecordUsage executes the cross-cutting concern of calculating billing cost (with off-peak discount and cached tokens),
// emitting telemetry metrics, injecting billing headers, and asynchronously recording audit logs.
func RecordUsage(c *gin.Context, params AuditRecordParams) float64 {
	var cost, savedCost float64
	var isOffPeak bool
	var offPeakDiscount float64 = 1.0
	keyGroup := getKeyGroup(c)
	success := params.StatusCode >= 200 && params.StatusCode < 400

	if success && billing.GlobalEngine != nil {
		cost, savedCost, _, isOffPeak, offPeakDiscount = billing.GlobalEngine.CalculateCostDetailedWithGroup(
			params.Model,
			keyGroup,
			params.PromptTokens,
			params.CompletionTokens,
			params.CachedTokens,
			time.Now(),
		)
	}

	totTokens := params.TotalTokens
	if totTokens == 0 {
		totTokens = params.PromptTokens + params.CompletionTokens
	}

	telemetry.GlobalMetrics.RecordRequestWithModel(
		params.Model,
		success,
		params.Duration,
		params.PromptTokens,
		params.CompletionTokens,
	)

	if storage.GlobalAsyncLogger != nil {
		tenantID := c.GetString("tenant_id")
		if tenantID == "" {
			tenantID = c.GetString(middleware.ContextKeyTenant)
		}
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:          middleware.GetTraceID(c),
			ChatID:           params.ChatID,
			Channel:          params.Channel,
			SessionID:        params.SessionID,
			APIKey:           getRequestAPIKey(c),
			TenantID:         tenantID,
			Model:            params.Model,
			PromptTokens:     params.PromptTokens,
			CompletionTokens: params.CompletionTokens,
			CachedTokens:     params.CachedTokens,
			TotalTokens:      totTokens,
			Cost:             cost,
			IsOffPeak:        isOffPeak,
			OffPeakDiscount:  offPeakDiscount,
			DurationMs:       params.Duration.Milliseconds(),
			TTFTMs:           params.TTFT.Milliseconds(),
			StatusCode:       params.StatusCode,
		})
	}

	if success && c != nil {
		c.Header("X-Airoute-Cost", fmt.Sprintf("%.6f", cost))
		if isOffPeak {
			c.Header("X-Airoute-Off-Peak", "true")
			c.Header("X-Airoute-Off-Peak-Discount", fmt.Sprintf("%.2f", offPeakDiscount))
		}
		if params.CachedTokens > 0 {
			c.Header("X-Airoute-Cached-Tokens", fmt.Sprintf("%d", params.CachedTokens))
			c.Header("X-Airoute-Saved-Cost", fmt.Sprintf("%.6f", savedCost))
		}
	}

	return cost
}

// RespondOpenAIError returns a standardized OpenAI API error JSON response.
func RespondOpenAIError(c *gin.Context, statusCode int, message, errType, code string) {
	if errType == "" {
		errType = "invalid_request_error"
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"code":    code,
		},
	})
}

// RespondAnthropicError returns a standardized Anthropic API error JSON response.
func RespondAnthropicError(c *gin.Context, statusCode int, message, errType string) {
	if errType == "" {
		errType = "invalid_request_error"
	}
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// RespondGeminiError returns a standardized Google Gemini API error JSON response.
func RespondGeminiError(c *gin.Context, statusCode int, message, status string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    statusCode,
			"message": message,
			"status":  status,
		},
	})
}

// InitSSEStream configures standard Server-Sent Events headers and flushes the writer.
func InitSSEStream(c *gin.Context) (http.Flusher, bool) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return flusher, ok
}

// getKeyGroup extracts key group name from context.
func getKeyGroup(c *gin.Context) string {
	if c == nil {
		return "default"
	}
	if kAny, exists := c.Get(middleware.ContextKeyAPIKeyConfig); exists {
		if k, ok := kAny.(*model.APIKeyConfig); ok && k.GroupName != "" {
			return k.GroupName
		}
	}
	return "default"
}

// getRequestAPIKey extracts incoming API key token string.
func getRequestAPIKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(middleware.ContextKeyAPIKey)
}

// recordFailedRequest audits an unsuccessful API dispatch with zero tokens and the designated status code.
func recordFailedRequest(c *gin.Context, sessionID, modelName string, dur time.Duration, statusCode int) {
	RecordUsage(c, AuditRecordParams{
		SessionID:        sessionID,
		Model:            modelName,
		PromptTokens:     0,
		CompletionTokens: 0,
		TotalTokens:      0,
		Duration:         dur,
		StatusCode:       statusCode,
	})
}

// getAPIKeyConfig returns APIKeyConfig metadata bound to the context.
func getAPIKeyConfig(c *gin.Context) *model.APIKeyConfig {
	if c == nil {
		return nil
	}
	if val, exists := c.Get(middleware.ContextKeyAPIKeyConfig); exists {
		if cfg, ok := val.(*model.APIKeyConfig); ok {
			return cfg
		}
	}
	return nil
}

// respondUpstreamDispatchError uniformly formats and audits upstream routing failures.
func respondUpstreamDispatchError(c *gin.Context, sessionID, modelName string, dur time.Duration, err error) {
	noUpstream := strings.Contains(err.Error(), "no upstream provider available")
	if noUpstream {
		recordFailedRequest(c, sessionID, modelName, dur, http.StatusNotFound)
		RespondOpenAIError(c, http.StatusNotFound, fmt.Sprintf("The model '%s' does not exist or has no active upstream providers configured.", modelName), "invalid_request_error", "model_not_found")
	} else {
		recordFailedRequest(c, sessionID, modelName, dur, http.StatusBadGateway)
		RespondOpenAIError(c, http.StatusBadGateway, err.Error(), "gateway_error", "upstream_failure")
	}
}
