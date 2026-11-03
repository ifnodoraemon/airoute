package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// traceIDPrefix marks a gateway-generated correlation id so it can never be
// confused with an upstream response id or a client-supplied traceparent.
const traceIDPrefix = "tr-"

// ensureTraceID stamps a correlation id on a usage record that has none.
//
// The fallback must be unique and opaque. It used to fall back to the session id,
// which is wrong on two counts: a session is a routing-affinity hint shared by
// every request in a conversation (so one session id appeared on dozens of audit
// rows and in any unique-index hypothetical), and it made the audit trail's
// trace_id column silently identical across different traces — a lookup by
// trace_id then returned a whole session rather than one request.
//
// A retried delivery keeps the id it was stamped with, because this helper only
// fills an empty value.
func ensureTraceID(rec *UsageLogRecord) {
	if rec == nil || rec.TraceID != "" {
		return
	}
	rec.TraceID = newFallbackTraceID()
}

// newFallbackTraceID builds "tr-<microseconds>-<6 random bytes in hex>".
//
// The microsecond component keeps the ids roughly sortable with the request time
// (and is what the audit UI already showed), while the random suffix makes two
// requests in the same microsecond distinct. crypto/rand is used because this is
// also a correlation key an operator may quote; a collision would merge two
// requests' evidence.
func newFallbackTraceID() string {
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		// crypto/rand failing is not a reason to leave trace_id empty (an empty
		// column is unsearchable); fall back to the timestamp alone.
		return fmt.Sprintf("%s%d", traceIDPrefix, time.Now().UnixMicro())
	}
	return fmt.Sprintf("%s%d-%s", traceIDPrefix, time.Now().UnixMicro(), hex.EncodeToString(suffix[:]))
}
