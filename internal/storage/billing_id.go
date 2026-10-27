package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
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

// BillingIDFromStreamEntry derives the idempotency key for a Redis Stream
// payload that was enqueued before billing_id existed.
//
// The stream entry id is assigned by Redis and is stable across every delivery
// of that entry (startup PEL recovery and the AutoClaim ticker reuse it), so a
// redelivery settles once. Minting a fresh random key per delivery — the
// previous behaviour — charged the same request twice.
func BillingIDFromStreamEntry(entryID string) string {
	return "stream-" + entryID
}

// BillingIDFromSpillLine derives the idempotency key for a spill file line
// written by a binary that predates billing_id.
//
// A spill file is only removed after a complete, successful replay, so the same
// line is re-read with the same 1-based position on the next attempt; deriving
// the key from (file, line) keeps a partially replayed file from charging its
// already-persisted prefix a second time.
func BillingIDFromSpillLine(file string, line int) string {
	return fmt.Sprintf("spill-%s-%d", file, line)
}

// DerivedBillingID is the last-resort idempotency key for a record that reached
// settlement without one (a caller that built a UsageLogRecord directly instead
// of going through AsyncLogger.Record).
//
// It must be deterministic: the same payload delivered twice has to produce the
// same key, otherwise a redelivery is a second charge. Every identity field is
// hashed, and created_at carries nanosecond precision, so two genuinely
// different requests cannot collapse into one charge. Only the transport
// boundaries (stream entry, spill line) can produce a more precise key; they run
// first.
//
// A record with no timestamp has no stable identity to hash: two different audit
// rows that happen to share every other field would collapse into one, and
// silently losing an audit row is worse than losing idempotency for a caller that
// never reports when it happened. Those records keep a fresh random key.
func DerivedBillingID(log *UsageLogRecord) string {
	if log == nil || log.CreatedAt.IsZero() {
		return NewBillingID()
	}
	hasher := sha256.New()
	_, _ = fmt.Fprintf(hasher, "%s|%q|%q|%q|%q|%q|%d|%d|%d|%d|%s",
		log.CreatedAt.UTC().Format(time.RFC3339Nano),
		log.TraceID, log.SessionID, log.APIKey, log.TenantID, log.Model,
		log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens,
		strconv.FormatFloat(log.Cost, 'g', -1, 64),
	)
	return "derived-" + hex.EncodeToString(hasher.Sum(nil))[:32]
}
