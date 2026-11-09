package api

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// AuditRecordParams encapsulates parameters required to record usage, calculate billing,
// and update system telemetry. Applying Separation of Concerns (SoC) and DRY.
type AuditRecordParams struct {
	Quota            *middleware.RequestQuota
	BillingID        string
	ReservationID    string
	SessionID        string
	ChatID           string
	Channel          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	// CacheCreationTokens is how many of PromptTokens were written into the
	// provider's prompt cache (Anthropic's cache_creation_input_tokens). They are
	// billed at the cache-write premium instead of the plain prompt rate.
	CacheCreationTokens int
	TotalTokens         int
	Duration            time.Duration
	TTFT                time.Duration
	StatusCode          int
	UsageSource         string // "upstream" (default) or "estimated" when tokens are locally approximated
	LiveTracked         bool   // true when streaming deltas already fed the live token meter
	ErrorMessage        string // populated for failed requests so the audit log explains the failure
	BillableUnits       int
	SkipBilling         bool

	// Caller attribution overrides for non-HTTP entrypoints.
	//
	// The MCP JSON-RPC transport has no gin context to derive the identity from,
	// yet its usage must be attributed (and settled) exactly like /v1/* traffic:
	// the old MCP path wrote a hand-built record with a fixed "mcp-session" key and
	// no pricing snapshot. Empty values fall back to the gin context, so HTTP
	// callers stay unchanged.
	APIKey    string
	TenantID  string
	KeyGroup  string
	TraceID   string
	KeyConfig *model.APIKeyConfig // carries the per-key TPM budget for consumption
}
