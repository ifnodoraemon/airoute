package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// The journal survives both process crashes and loss of the Redis queue. It is
// retired only with the audit insert, independently of the wallet/hold commit.
func (r *Repository) PendBillingRecord(ctx context.Context, rec *UsageLogRecord) error {
	copy := rec.CloneForPipeline()
	normalizeUsageLog(copy)
	payload, err := json.Marshal(copy)
	if err != nil {
		return err
	}
	return r.reservationTransaction(ctx, func(tx *Tx) error {
		var key string
		err := tx.QueryRowContext(ctx, `SELECT api_key FROM billing_reservations WHERE id = ?`, rec.ReservationID).Scan(&key)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if _, err := r.lockBillingAccount(ctx, tx, key); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO billing_usage_journal (billing_id, payload) VALUES (?, ?) ON CONFLICT (billing_id) DO NOTHING`, rec.BillingID, string(payload)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE billing_reservations SET amount_units = CASE WHEN amount_units < ? THEN ? ELSE amount_units END, pending = TRUE, billing_id = ? WHERE id = ? AND api_key = ?`, ToUnits(rec.Cost), ToUnits(rec.Cost), rec.BillingID, rec.ReservationID, copy.APIKey)
		return err
	})
}

func retireBillingJournal(tx *Tx, logs []*UsageLogRecord) error {
	ids := make(map[string]bool)
	for _, rec := range logs {
		if rec != nil && rec.ReservationID != "" && rec.BillingID != "" {
			ids[rec.BillingID] = true
		}
	}
	ordered := sortedMapKeys(ids)
	for start := 0; start < len(ordered); start += 100 {
		batch := ordered[start:min(start+100, len(ordered))]
		args := make([]any, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		in := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		if _, err := tx.Exec(`DELETE FROM billing_usage_journal WHERE billing_id IN (`+in+`)`, args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) recoverBillingJournal(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `SELECT payload FROM billing_usage_journal ORDER BY created_at, billing_id LIMIT 100`)
	if err != nil {
		return err
	}
	var records []*UsageLogRecord
	for rows.Next() {
		var payload string
		var rec UsageLogRecord
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(payload), &rec); err != nil {
			rows.Close()
			return err
		}
		records = append(records, &rec)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(records) == 0 {
		return err
	}
	return r.BatchRecordUsageLogsContext(ctx, records)
}

func (b *SettlementBatcher) recoverReservations() {
	defer b.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
		}
		ctx, cancel := context.WithTimeout(context.Background(), logWriteTimeout)
		err := b.repo.recoverBillingJournal(ctx)
		b.repo.observeBillingReservations(ctx)
		cancel()
		if err != nil {
			telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-journal-recovery", 30*time.Second,
				"billing journal recovery deferred", "error", err.Error())
		}
	}
}

func (r *Repository) observeBillingReservations(ctx context.Context) {
	var held int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_units), 0) FROM billing_reservations`).Scan(&held); err == nil {
		telemetry.GlobalMetrics.BillingReservedUnits.Store(held)
	}
}

func commitUsageAudit(tx *Tx, logs []*UsageLogRecord) error {
	if err := retireBillingJournal(tx, logs); err != nil {
		return err
	}
	return tx.Commit()
}
