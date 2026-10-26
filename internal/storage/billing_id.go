package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

// billingFallbackSeq only increments when crypto/rand is unavailable. It keeps
// the key unique instead of panicking, because losing a billing row is worse
// than a predictable-but-unique key.
var billingFallbackSeq atomic.Uint64

// NewBillingID returns a server-generated idempotency key for one billable
// usage row.
//
// The key is generated once, when the record enters the pipeline, and travels
// with it: in-memory buffer -> Redis stream -> database. A redelivery of the
// same record (commit succeeded but XACK failed, consumer crash, AutoClaim)
// therefore reuses the key, and the unique index on usage_logs.billing_id turns
// the duplicate write into a no-op instead of a second wallet deduction.
//
// It must never be derived from client-supplied data: X-Request-ID/traceparent
// are attacker-controlled, and a client could otherwise ask to be billed once
// by replaying a header.
func NewBillingID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("bill-%d-%d", time.Now().UnixNano(), billingFallbackSeq.Add(1))
	}
	return "bill-" + hex.EncodeToString(b[:])
}
