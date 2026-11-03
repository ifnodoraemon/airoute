package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func (h *AdminHandler) syncStripeOrder(ctx context.Context, number string, cfg stripeConfig, force bool, knownSession string) (result *storage.RechargeOrderRecord, retErr error) {
	o, err := h.repo.ClaimRecharge(number, cfg.Mode, force)
	if err != nil {
		return nil, err
	}
	next := time.Now().Add(15 * time.Second).Unix()
	defer func() {
		message := ""
		if retErr != nil {
			message = retErr.Error()
		}
		if err := h.repo.ReleaseRecharge(number, o.SyncVersion, next, message); err != nil {
			telemetry.ScopedLogger("payment").Error("failed to release payment lease", "order_no", number, "error", err.Error())
		}
	}()
	var s stripeSession
	if knownSession != "" {
		if o.StripeSessionID != "" && o.StripeSessionID != knownSession {
			return o, storage.ErrPaymentConflict
		}
		s, err = cfg.getSession(ctx, knownSession)
	} else if o.StripeSessionID == "" {
		found := false
		if o.LastSyncError != "" {
			s, found, err = cfg.recoverSession(ctx, o)
			if err != nil {
				return o, err
			}
		}
		// Never recreate an ambiguous attempt after Stripe's idempotency retention
		// or with an expires_at that is no longer valid for session creation.
		if !found && o.ExpiresAt < time.Now().Add(31*time.Minute).Unix() {
			_, err := h.repo.GetDB().Exec(`UPDATE recharge_orders SET status='failed',updated_at=CURRENT_TIMESTAMP WHERE order_no=? AND sync_version=? AND credited_cents=0`, number, o.SyncVersion)
			if err != nil {
				return nil, err
			}
			next = time.Now().Add(24 * time.Hour).Unix()
			return h.repo.GetRechargeOrder(number)
		}
		if !found {
			s, err = cfg.createSession(ctx, o)
		}
		// Stripe replays the original response body for an idempotent create;
		// retrieve current state even when that cached body still says unpaid.
		if err == nil && s.ID != "" {
			s, err = cfg.getSession(ctx, s.ID)
		}
	} else {
		s, err = cfg.getSession(ctx, o.StripeSessionID)
	}
	if err != nil {
		return o, err
	}
	if s.ID == "" || (o.StripeSessionID != "" && o.StripeSessionID != s.ID) || s.Reference != o.OrderNo || s.Mode != "payment" || s.LiveMode == nil || *s.LiveMode != (cfg.Mode == "live") || s.Currency != "cny" || s.Amount != o.Cents() || s.ExpiresAt <= 0 {
		return o, storage.ErrPaymentConflict
	}
	status := "pending"
	if s.PaymentStatus == "paid" {
		status = "paid"
	} else if s.PaymentStatus != "unpaid" {
		return o, storage.ErrPaymentConflict
	} else if s.Status == "expired" {
		status = "expired"
	}
	if status == "pending" && s.Status == "complete" && s.PaymentIntent != "" {
		var pi struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
			LiveMode *bool  `json:"livemode"`
		}
		if err := cfg.request(ctx, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(string(s.PaymentIntent)), nil, "", &pi); err != nil {
			return o, err
		}
		if pi.ID != string(s.PaymentIntent) || pi.Amount != s.Amount || pi.Currency != s.Currency || pi.LiveMode == nil || *pi.LiveMode != (cfg.Mode == "live") {
			return o, storage.ErrPaymentConflict
		}
		if pi.Status == "requires_payment_method" || pi.Status == "canceled" {
			status = "failed"
		}
	}
	if s.Status != "open" && s.Status != "complete" && s.Status != "expired" {
		return o, storage.ErrPaymentConflict
	}
	if s.Status == "open" {
		u, err := url.Parse(s.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return o, fmt.Errorf("invalid Stripe checkout URL")
		}
	}
	snapshot := storage.PaymentSnapshot{SessionID: s.ID, PaymentIntentID: string(s.PaymentIntent), Mode: cfg.Mode, Currency: "CNY", Status: status, AmountCents: s.Amount, CheckoutURL: s.URL, ExpiresAt: s.ExpiresAt}
	if status == "paid" {
		if snapshot.PaymentIntentID == "" {
			return o, storage.ErrPaymentConflict
		}
		snapshot.Adjustments, err = cfg.adjustments(ctx, snapshot.PaymentIntentID)
		if err != nil {
			return o, err
		}
	}
	updated, err := h.repo.ApplyPaymentSnapshot(number, o.SyncVersion, snapshot)
	if err != nil {
		return o, err
	}
	if updated.CreditedCents != o.CreditedCents || updated.ReversedCents != o.ReversedCents {
		h.syncDataPlane()
	}
	next = time.Now().Add(5 * time.Second).Unix()
	if updated.CreditedCents > 0 {
		next = time.Now().Add(time.Hour).Unix()
	}
	return updated, nil
}

func (h *AdminHandler) processStripeEvent(ctx context.Context, e storage.StripeEventRecord, cfg stripeConfig) error {
	var number, knownSession string
	if strings.HasPrefix(e.Type, "checkout.session.") {
		s, err := cfg.getSession(ctx, e.ObjectID)
		if err != nil {
			return err
		}
		number = s.Reference
		knownSession = s.ID
	} else {
		path := "/v1/charges/"
		if strings.HasPrefix(e.Type, "refund.") {
			path = "/v1/refunds/"
		} else if strings.HasPrefix(e.Type, "charge.dispute.") {
			path = "/v1/disputes/"
		}
		var object struct {
			PaymentIntent stripeReference `json:"payment_intent"`
			Charge        stripeReference `json:"charge"`
			LiveMode      *bool           `json:"livemode"`
		}
		if err := cfg.request(ctx, http.MethodGet, path+url.PathEscape(e.ObjectID), nil, "", &object); err != nil {
			return err
		}
		if object.LiveMode != nil && *object.LiveMode != (cfg.Mode == "live") {
			return storage.ErrPaymentConflict
		}
		if object.PaymentIntent == "" && object.Charge != "" {
			if err := cfg.request(ctx, http.MethodGet, "/v1/charges/"+url.PathEscape(string(object.Charge)), nil, "", &object); err != nil {
				return err
			}
			if object.LiveMode == nil || *object.LiveMode != (cfg.Mode == "live") {
				return storage.ErrPaymentConflict
			}
		}
		if object.PaymentIntent == "" {
			return fmt.Errorf("Stripe object has no payment intent")
		}
		o, err := h.repo.FindRechargeByPaymentIntent(string(object.PaymentIntent))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if o != nil {
			number = o.OrderNo
			knownSession = o.StripeSessionID
		} else {
			// Refund/dispute notifications may beat the checkout notification.
			var list struct {
				Data []stripeSession `json:"data"`
				More bool            `json:"has_more"`
			}
			if err := cfg.request(ctx, http.MethodGet, "/v1/checkout/sessions", url.Values{"payment_intent": {string(object.PaymentIntent)}, "limit": {"100"}}, "", &list); err != nil {
				return err
			}
			if list.More {
				return fmt.Errorf("ambiguous Stripe payment intent sessions")
			}
			for _, s := range list.Data {
				if string(s.PaymentIntent) != string(object.PaymentIntent) {
					return storage.ErrPaymentConflict
				}
				candidate, err := h.repo.GetRechargeOrder(s.Reference)
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return err
				}
				if candidate != nil {
					number = candidate.OrderNo
					knownSession = s.ID
					break
				}
			}
		}
	}
	if number == "" {
		return nil
	}
	_, err := h.repo.GetRechargeOrder(number)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = h.syncStripeOrder(ctx, number, cfg, true, knownSession)
	return err
}

func (h *AdminHandler) reconcileStripePayments(ctx context.Context) {
	cfg, err := loadStripeConfig()
	if err != nil || !cfg.enabled() {
		return
	}
	if err := h.repo.BindPaymentMode(cfg.Mode); err != nil {
		telemetry.ScopedLogger("payment").Error("payment environment mismatch", "error", err.Error())
		return
	}
	events, err := h.repo.PendingStripeEvents(cfg.Mode, 20)
	if err != nil {
		telemetry.ScopedLogger("payment").Error("cannot read pending Stripe notifications", "error", err.Error())
		return
	}
	for _, e := range events {
		if ctx.Err() != nil {
			return
		}
		op, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := h.processStripeEvent(op, e, cfg)
		cancel()
		if finishErr := h.repo.FinishStripeEvent(e.ID, err); finishErr != nil {
			telemetry.ScopedLogger("payment").Error("cannot record Stripe notification result", "event_id", e.ID, "error", finishErr.Error())
		}
		if err != nil {
			telemetry.ScopedLogger("payment").Warn("Stripe notification awaits recovery", "event_id", e.ID, "error", err.Error())
		}
	}
	numbers, err := h.repo.DueRechargeOrders(cfg.Mode, 20)
	if err != nil {
		telemetry.ScopedLogger("payment").Error("cannot read payment recovery queue", "error", err.Error())
		return
	}
	for _, number := range numbers {
		if ctx.Err() != nil {
			return
		}
		op, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, err := h.syncStripeOrder(op, number, cfg, false, "")
		cancel()
		if err != nil && !errors.Is(err, storage.ErrPaymentBusy) {
			telemetry.ScopedLogger("payment").Warn("payment reconciliation pending", "order_no", number, "error", err.Error())
		}
	}
}

// RunPaymentReconciler is canceled and joined by server shutdown. Database
// leases coordinate replicas; no process-local lock is relied on for money.
func (h *AdminHandler) RunPaymentReconciler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		cycle, cancel := context.WithTimeout(ctx, 45*time.Second)
		h.reconcileStripePayments(cycle)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
