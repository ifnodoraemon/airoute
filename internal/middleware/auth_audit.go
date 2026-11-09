package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// auditAuthFailure logs a client-side admission rejection (insufficient balance,
// disabled or throttled key) into the audit table so 402/403/429 denials stay
// visible to operators.
//
// A 401 is deliberately not persisted. It means the caller never authenticated:
// no key matched, no request reached routing, nothing was billed. The row could
// only carry an empty model and channel, so it added noise to the request log and
// skewed the error rate. The access log still records the same trace id with its
// path, peer address and user agent.
//
// The opposite direction is unaffected: a 401/403 returned by an upstream
// provider is a channel failure on a real, billable request and is persisted
// with the upstream status text, because that means our own credential to the
// provider is broken.
func auditAuthFailure(c *gin.Context, rawKey string, statusCode int, errMsg string) {
	if statusCode == http.StatusUnauthorized {
		return
	}
	// Record the digest, exactly as a successful call does: usage_logs.api_key is
	// the join key back to api_keys, and a digest is not a usable credential. A
	// value that never matched a row is not recorded (the 401 early-return above
	// covers it), so no fragment of an unrecognised submission is persisted.
	auditDigest := ""
	if rawKey != "" {
		auditDigest = storage.HashAPIKey(rawKey)
	}
	recordAdmissionRejection(c, auditDigest, statusCode, errMsg)
}

// auditDenial records a denial raised after authentication has already resolved the
// credential: the request context holds the stored digest (ContextKeyAPIKey), so it
// is written as-is. Hashing it again would produce a row that joins to no key.
//
// Rate limiting runs behind AuthMiddleware, so routing its 429s here is what makes
// the auditAuthFailure contract ("402/403/429 stay visible") actually hold.
func auditDenial(c *gin.Context, denial *AccessDenial) {
	if denial == nil {
		return
	}
	auditDigest := ""
	if c != nil {
		auditDigest = c.GetString(ContextKeyAPIKey)
	}
	recordAdmissionRejection(c, auditDigest, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
}

// recordAdmissionRejection is the single writing funnel for admission rejections:
// every 402/403/429 exit goes through here, so a newly added denial path cannot
// silently skip the audit trail. 401 is filtered by auditAuthFailure.
func recordAdmissionRejection(c *gin.Context, auditDigest string, statusCode int, errMsg string) {
	if c == nil || storage.GlobalAsyncLogger == nil {
		return
	}
	traceID := c.GetString("trace_id")
	if traceID == "" {
		traceID = c.GetHeader("x-request-id")
	}
	storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
		TraceID:      traceID,
		APIKey:       auditDigest,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		UsageSource:  "gateway_auth",
	})
}

// The resolved digest is already the ledger join key. Never hash it again or
// make it replayable as a public bearer credential.
func auditResolvedKeyFailure(c *gin.Context, digest string, statusCode int, message string) {
	if statusCode != http.StatusUnauthorized {
		recordAdmissionRejection(c, digest, statusCode, message)
	}
}
