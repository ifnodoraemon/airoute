package storage

import (
	"context"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func (r *Repository) settleChargesOnce(ctx context.Context, logs []*UsageLogRecord) error {
	pending, duplicates := prepareSettlementLogs(logs)
	if len(pending) == 0 {
		for _, rec := range logs {
			if rec != nil && rec.ReservationID != "" {
				return r.reservationTransaction(ctx, func(tx *Tx) error { return finishBillingReservations(tx, logs) })
			}
		}
		return nil
	}
	start := time.Now()
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
		telemetry.SettlementDurationSeconds.Observe(time.Since(start).Seconds())
	}()
	if err := r.lockSettlementAccounts(tx, pending); err != nil {
		return err
	}
	fresh, err := r.insertSettlementRows(tx, pending)
	if err != nil {
		return err
	}
	effects := &settlementEffects{}
	if err := r.applyNewSettlementCharges(tx, pending, fresh, effects); err != nil {
		return err
	}
	if err := finishBillingReservations(tx, logs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	effects.flush()
	for _, rec := range pending {
		rec.Settled = true
	}
	for _, rec := range duplicates {
		rec.Settled = true
	}
	return nil
}

// Collapse duplicates before aggregation. Only the first occurrence of a
// billing ID contributes money, and no copy is marked settled before commit.
func prepareSettlementLogs(logs []*UsageLogRecord) (pending, duplicates []*UsageLogRecord) {
	seen := make(map[string]bool, len(logs))
	for _, rec := range logs {
		if rec == nil || rec.Settled {
			continue
		}
		if rec.PricingSnapshot != nil && rec.Cost > 0 {
			if err := ValidatePriceCurrency(&ModelPriceRecord{Currency: rec.PricingSnapshot.Currency}); err != nil {
				rejectUnbillableCharge(rec, err)
				continue
			}
		}
		if rec.Cost <= 0 && rec.TotalTokens <= 0 {
			rec.Settled = true
			continue
		}
		if rec.BillingID == "" {
			rec.BillingID = DerivedBillingID(rec)
		}
		if seen[rec.BillingID] {
			duplicates = append(duplicates, rec)
			continue
		}
		seen[rec.BillingID] = true
		pending = append(pending, rec)
	}
	return
}

func (r *Repository) applyNewSettlementCharges(tx *Tx, pending []*UsageLogRecord, fresh map[string]struct{}, effects *settlementEffects) error {
	keyCost, orphanCost := map[string]float64{}, map[string]float64{}
	keyTokens, keyTenant := map[string]int64{}, map[string]string{}
	for _, rec := range pending {
		if _, exists := fresh[rec.BillingID]; !exists {
			continue
		}
		if rec.APIKey != "" {
			keyCost[rec.APIKey] += rec.Cost
			keyTokens[rec.APIKey] += int64(rec.TotalTokens)
			if rec.TenantID != "" {
				keyTenant[rec.APIKey] = rec.TenantID
			}
		} else if rec.TenantID != "" && rec.Cost > 0 {
			orphanCost[rec.TenantID] += rec.Cost
		}
	}
	return r.applyChargeAggregates(tx, keyCost, keyTokens, keyTenant, orphanCost, effects)
}
