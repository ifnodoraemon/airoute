package storage

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Settlement batching (group commit) for the data plane.
//
// The audit of the synchronous settlement path measured a flat ~2.8-3.5k
// charges/s with latency growing linearly with concurrency (p99 217 ms at
// 128 concurrent settlements on SQLite WAL): the cost is one database
// transaction per request, not the transaction machinery. Merging K charges
// into one transaction measured 3.2x (K=10), 6.1x (K=100) and 9.9x (K=500)
// the throughput of the K=1 path, because the per-record statement count and
// commit/fsync are amortised.
//
// This file implements that merge without changing settlement semantics:
//
//   - each request still waits for *its* charge to be durable, up to a short
//     deadline (default 300 ms), so "money moves at request completion" holds
//     for the normal case;
//   - if the wait expires (writer saturated), the caller is told to defer:
//     the record is already travelling through the at-least-once audit
//     pipeline, which settles it later. The ledger's unique billing_id makes
//     a batched settlement and a pipeline settlement of the same record
//     apply exactly once.
//
// The batcher never invents a different money path: it calls the same
// settleCharges transaction the synchronous and async paths use, so all of the
// existing rules (idempotency, admin exemption, overdraft floor, currency
// validation) still apply.
const (
	defaultSettlementBatchSize = 100
	defaultSettlementLinger    = 2 * time.Millisecond
	defaultSettlementQueueSize = 8192
	defaultSettlementWait      = 300 * time.Millisecond
	// settlementFlushTimeout bounds one merged transaction. It must stay well
	// below the request-side wait so a stalled database turns into "deferred"
	// charges instead of wedged request goroutines.
	settlementFlushTimeout = 5 * time.Second
)

// cloneForSettlement returns the shallow copy the group-commit flusher is
// allowed to mutate. The request goroutine keeps ownership of the original, so
// marking Settled (or deriving a key) on the copy cannot race with a caller that
// timed out and is still using its record. The pricing snapshot is shared and
// read-only.
func (r *UsageLogRecord) cloneForSettlement() *UsageLogRecord {
	if r == nil {
		return nil
	}
	clone := *r
	return &clone
}

// CloneForPipeline returns a shallow copy of the record for the audit pipeline.
//
// The settlement batcher mutates the copy it is given (derived billing id,
// Settled flag), and the pipeline mutates the record it receives (normalisation
// on insert). Handing each path its own copy keeps the request goroutine's
// record single-owner, so "enqueue the audit row first, then settle" cannot race
// on those fields. The pricing snapshot is shared and read-only.
func (r *UsageLogRecord) CloneForPipeline() *UsageLogRecord {
	if r == nil {
		return nil
	}
	clone := *r
	return &clone
}

// settleJob is one charge waiting for the next flush.
type settleJob struct {
	rec  *UsageLogRecord
	done chan error
}

// SettlementBatcher merges concurrent charges into shared database transactions.
type SettlementBatcher struct {
	repo     *Repository
	jobs     chan settleJob
	maxBatch int
	linger   time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	stopped  atomic.Bool
	wg       sync.WaitGroup
}

// globalSettlementBatcher is installed once at startup by InitSettlementBatcher.
var globalSettlementBatcher *SettlementBatcher

// InitSettlementBatcher starts the group-commit worker and installs it as the
// process-wide settlement path. A nil repository disables batching (callers fall
// back to synchronous settlement).
func InitSettlementBatcher(repo *Repository) *SettlementBatcher {
	if repo == nil {
		return nil
	}
	b := &SettlementBatcher{
		repo:     repo,
		jobs:     make(chan settleJob, envPositiveInt("GATEWAY_SETTLEMENT_QUEUE", defaultSettlementQueueSize)),
		maxBatch: envPositiveInt("GATEWAY_SETTLEMENT_BATCH_SIZE", defaultSettlementBatchSize),
		linger:   envMilliseconds("GATEWAY_SETTLEMENT_LINGER_MS", defaultSettlementLinger),
		stopCh:   make(chan struct{}),
	}
	globalSettlementBatcher = b
	b.wg.Add(1)
	go b.run()
	return b
}

// envMilliseconds reads a positive millisecond override as a duration.
func envMilliseconds(name string, dflt time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return dflt
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return dflt
	}
	return time.Duration(value) * time.Millisecond
}

// SettlementWaitTimeout exposes the request-side wait so handlers can bound the
// same deadline the batcher uses (one env var, not two).
func SettlementWaitTimeout() time.Duration { return settlementWaitTimeout() }

// settlementWaitTimeout is how long a request waits for its charge to commit.
// It is deliberately shorter than settlementTimeout (the old single-record
// deadline): a wait that long only ever happened when the writer was already
// saturated, and by then the caller should defer to the pipeline rather than
// hold the response.
func settlementWaitTimeout() time.Duration {
	return envMilliseconds("GATEWAY_SETTLEMENT_WAIT_MS", defaultSettlementWait)
}

// SettleChargeBatched hands one charge to the group-commit writer and waits, up
// to a short deadline, for it to be durable.
//
// A non-nil error means "this charge was not confirmed durable in time". It is
// not a billing failure: the caller's record is already on the at-least-once
// audit pipeline, which retries the settlement. Callers keep their previous
// behaviour on error (count it as deferred, never as lost).
func SettleChargeBatched(ctx context.Context, rec *UsageLogRecord) error {
	b := globalSettlementBatcher
	if b == nil || b.stopped.Load() {
		// Tests, CLI subcommands and shutdown: keep the direct path.
		return SettleChargeNow(ctx, rec)
	}
	return b.enqueue(ctx, rec)
}

// enqueue adds a charge to the next flush and waits for the outcome.
func (b *SettlementBatcher) enqueue(ctx context.Context, rec *UsageLogRecord) error {
	if rec == nil {
		return nil
	}
	// Pin the idempotency key on the caller's record before anything else: the
	// flusher settles a copy, and if that copy derived a key of its own the
	// caller's later audit delivery would mint a different one and charge twice.
	if rec.BillingID == "" {
		rec.BillingID = NewBillingID()
	}
	job := settleJob{rec: rec.cloneForSettlement(), done: make(chan error, 1)}

	select {
	case b.jobs <- job:
		telemetry.GlobalMetrics.SettlementQueueDepth.Store(int64(len(b.jobs)))
	default:
		// The queue is full: the writer cannot keep up. Refuse to block the
		// request goroutine (and never grow memory without bound); the charge
		// stays deferred to the audit pipeline.
		telemetry.GlobalMetrics.SettlementWaitTimeouts.Add(1)
		countDeferredSettlement(ErrSettlementBacklog)
		return ErrSettlementBacklog
	}

	select {
	case err := <-job.done:
		if err != nil {
			countDeferredSettlement(err)
			return err
		}
		// The flusher settled the copy; mark the caller's record so the audit
		// row records "settled" and the pipeline does not probe the ledger again.
		rec.Settled = true
		return nil
	case <-ctx.Done():
		// The flush may still commit this charge later; the ledger key makes the
		// later pipeline settlement a no-op in that case.
		telemetry.GlobalMetrics.SettlementWaitTimeouts.Add(1)
		countDeferredSettlement(ctx.Err())
		return ctx.Err()
	}
}

// ErrSettlementBacklog means the group-commit queue was full, so the charge was
// left to the asynchronous audit pipeline instead of stalling the request.
var ErrSettlementBacklog = settlementBacklogError{}

type settlementBacklogError struct{}

func (settlementBacklogError) Error() string {
	return "settlement backlog: charge deferred to the async pipeline"
}

// run is the group-commit loop.
func (b *SettlementBatcher) run() {
	defer b.wg.Done()

	batch := make([]settleJob, 0, b.maxBatch)
	// flush applies the collected jobs in one transaction and wakes every waiter.
	flush := func() {
		if len(batch) == 0 {
			return
		}
		b.flush(batch)
		batch = batch[:0]
		telemetry.GlobalMetrics.SettlementQueueDepth.Store(int64(len(b.jobs)))
	}

	ticker := time.NewTicker(b.linger)
	defer ticker.Stop()

	for {
		select {
		case job, ok := <-b.jobs:
			if !ok {
				flush()
				return
			}
			batch = append(batch, job)
			if len(batch) >= b.maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.stopCh:
			// Drain whatever is already queued, then commit it. Records that
			// arrive after this point take the direct path (stopped is set before
			// stopCh is closed in Stop).
			for {
				select {
				case job := <-b.jobs:
					batch = append(batch, job)
					if len(batch) >= b.maxBatch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// flush settles one batch and reports the shared outcome to every waiter.
func (b *SettlementBatcher) flush(batch []settleJob) {
	records := make([]*UsageLogRecord, 0, len(batch))
	for _, job := range batch {
		records = append(records, job.rec)
	}

	ctx, cancel := context.WithTimeout(context.Background(), settlementFlushTimeout)
	err := b.repo.settleCharges(ctx, records)
	cancel()

	// The transaction-level duration is observed inside settleCharges; here only
	// the batch shape is recorded, so a saturation shows up as both slower
	// transactions and larger batches.
	telemetry.SettlementBatchSize.Observe(float64(len(records)))
	telemetry.GlobalMetrics.SettlementBatches.Add(1)
	telemetry.GlobalMetrics.SettlementBatchRecords.Add(uint64(len(records)))

	for _, job := range batch {
		// Buffered channel: a waiter that already gave up (ctx expired) must not
		// block the flusher.
		select {
		case job.done <- err:
		default:
		}
	}
}

// Stop flushes remaining charges and stops the worker. It must be called once at
// shutdown; afterwards SettleChargeBatched falls back to direct settlement.
func (b *SettlementBatcher) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() {
		b.stopped.Store(true)
		close(b.stopCh)
	})
	b.wg.Wait()
}
