package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

type stripeReference string

func (s *stripeReference) UnmarshalJSON(body []byte) error {
	if string(body) == "null" {
		return nil
	}
	var id string
	if err := json.Unmarshal(body, &id); err == nil {
		*s = stripeReference(id)
		return nil
	}
	var object struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &object); err != nil {
		return err
	}
	*s = stripeReference(object.ID)
	return nil
}

type stripeSession struct {
	ID            string          `json:"id"`
	Reference     string          `json:"client_reference_id"`
	Mode          string          `json:"mode"`
	LiveMode      *bool           `json:"livemode"`
	Status        string          `json:"status"`
	PaymentStatus string          `json:"payment_status"`
	Amount        int64           `json:"amount_total"`
	Currency      string          `json:"currency"`
	URL           string          `json:"url"`
	ExpiresAt     int64           `json:"expires_at"`
	PaymentIntent stripeReference `json:"payment_intent"`
}

type stripeAPIError struct {
	Status  int
	Message string
}

func (e *stripeAPIError) Error() string {
	return fmt.Sprintf("Stripe HTTP %d: %s", e.Status, e.Message)
}

func (cfg stripeConfig) request(ctx context.Context, method, path string, form url.Values, key string, target any) error {
	var body io.Reader
	if method == http.MethodGet && len(form) > 0 {
		path += "?" + form.Encode()
	} else if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.APIBase+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	req.Header.Set("Stripe-Version", StripeAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	// Do not follow a redirect carrying a server credential to another origin.
	client := *http.DefaultClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Stripe request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 2*1024*1024 {
		return fmt.Errorf("Stripe response exceeds size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var envelope struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &envelope)
		return &stripeAPIError{Status: resp.StatusCode, Message: envelope.Error.Message}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("invalid Stripe response: %w", err)
	}
	return nil
}

func (cfg stripeConfig) createSession(ctx context.Context, o *storage.RechargeOrderRecord) (stripeSession, error) {
	form := url.Values{"success_url": {o.SuccessURL}, "cancel_url": {o.CancelURL}, "payment_method_types[0]": {"card"}, "mode": {"payment"}, "adaptive_pricing[enabled]": {"false"},
		"client_reference_id": {o.OrderNo}, "expires_at": {strconv.FormatInt(o.ExpiresAt, 10)}, "metadata[airoute_order]": {o.OrderNo}, "payment_intent_data[metadata][airoute_order]": {o.OrderNo},
		"line_items[0][price_data][currency]": {"cny"}, "line_items[0][price_data][unit_amount]": {strconv.FormatInt(o.Cents(), 10)},
		"line_items[0][price_data][product_data][name]": {fmt.Sprintf("Airoute 钱包充值 (¥%.2f)", o.Amount)}, "line_items[0][quantity]": {"1"}}
	var s stripeSession
	err := cfg.request(ctx, http.MethodPost, "/v1/checkout/sessions", form, "airoute-checkout-"+o.OrderNo, &s)
	return s, err
}

func (cfg stripeConfig) getSession(ctx context.Context, id string) (stripeSession, error) {
	var s stripeSession
	err := cfg.request(ctx, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(id), nil, "", &s)
	return s, err
}

// Recover an indeterminate create (including a cached Stripe 500) by the
// server-owned reference. This also works after the idempotency retention ends.
func (cfg stripeConfig) recoverSession(ctx context.Context, o *storage.RechargeOrderRecord) (stripeSession, bool, error) {
	var found stripeSession
	cursor := ""
	for page := 0; page < 100; page++ {
		query := url.Values{"created[gte]": {strconv.FormatInt(o.CreatedAt.Add(-time.Minute).Unix(), 10)}, "created[lte]": {strconv.FormatInt(o.ExpiresAt, 10)}, "limit": {"100"}}
		if cursor != "" {
			query.Set("starting_after", cursor)
		}
		var list struct {
			Data []stripeSession `json:"data"`
			More bool            `json:"has_more"`
		}
		if err := cfg.request(ctx, http.MethodGet, "/v1/checkout/sessions", query, "", &list); err != nil {
			return found, false, err
		}
		for _, s := range list.Data {
			if s.Reference == o.OrderNo {
				if found.ID != "" && found.ID != s.ID {
					return found, false, fmt.Errorf("multiple Stripe sessions share an order reference")
				}
				found = s
			}
		}
		if !list.More {
			return found, found.ID != "", nil
		}
		if len(list.Data) == 0 || list.Data[len(list.Data)-1].ID == cursor || page == 99 {
			return found, false, fmt.Errorf("invalid Stripe session pagination")
		}
		cursor = list.Data[len(list.Data)-1].ID
	}
	return found, false, fmt.Errorf("Stripe session recovery exceeded page limit")
}

func (cfg stripeConfig) adjustments(ctx context.Context, pi string) ([]storage.PaymentAdjustment, error) {
	var result []storage.PaymentAdjustment
	for _, kind := range []string{"refund", "dispute"} {
		cursor := ""
		for page := 0; page < 100; page++ {
			query := url.Values{"payment_intent": {pi}, "limit": {"100"}}
			if cursor != "" {
				query.Set("starting_after", cursor)
			}
			var list struct {
				Data []struct {
					storage.PaymentAdjustment
					LiveMode *bool `json:"livemode"`
				} `json:"data"`
				More bool `json:"has_more"`
			}
			if err := cfg.request(ctx, http.MethodGet, "/v1/"+kind+"s", query, "", &list); err != nil {
				return nil, err
			}
			for _, entry := range list.Data {
				a := entry.PaymentAdjustment
				a.Kind = kind
				a.LiveMode = cfg.Mode == "live"
				// Refunds have no livemode attribute; the authenticated, PI-scoped
				// query and verified Session supply that binding. Disputes do.
				if kind == "dispute" && (entry.LiveMode == nil || *entry.LiveMode != a.LiveMode) {
					return nil, storage.ErrPaymentConflict
				}
				if a.PaymentIntentID == "" {
					a.PaymentIntentID = pi
				}
				result = append(result, a)
			}
			if !list.More {
				break
			}
			if len(list.Data) == 0 || list.Data[len(list.Data)-1].ID == cursor || page == 99 {
				return nil, fmt.Errorf("invalid Stripe adjustment pagination")
			}
			cursor = list.Data[len(list.Data)-1].ID
		}
	}
	return result, nil
}
