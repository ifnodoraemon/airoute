package controlplane

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// The API and snapshot webhook destination must use the same tested version.
const StripeAPIVersion = "2026-10-28.endive"

type stripeConfig struct {
	Key, Secret, Mode, APIBase, PublicURL, SuccessURL string
	MinAmount                                         float64
}

func loadStripeConfig() (stripeConfig, error) {
	c := stripeConfig{Key: strings.TrimSpace(os.Getenv("STRIPE_API_KEY")), Secret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")), Mode: strings.TrimSpace(os.Getenv("STRIPE_MODE")), MinAmount: 10, APIBase: "https://api.stripe.com"}
	if c.Mode == "" {
		c.Mode = "live"
	}
	if c.Mode != "live" && c.Mode != "test" {
		return c, fmt.Errorf("STRIPE_MODE must be live or test")
	}
	if raw := strings.TrimSpace(os.Getenv("STRIPE_MIN_RECHARGE_CNY")); raw != "" {
		amount, err := strconv.ParseFloat(raw, 64)
		if err != nil || !storage.ValidRechargeAmount(amount) {
			return c, fmt.Errorf("STRIPE_MIN_RECHARGE_CNY must be a positive CNY amount with at most two decimals")
		}
		c.MinAmount = amount
	}
	if c.Key == "" && c.Secret == "" {
		return c, nil
	}
	if c.Key == "" || c.Secret == "" {
		return c, fmt.Errorf("Stripe API key and webhook signing secret must both be configured")
	}
	if (!strings.HasPrefix(c.Key, "sk_"+c.Mode+"_") && !strings.HasPrefix(c.Key, "rk_"+c.Mode+"_")) || !strings.HasPrefix(c.Secret, "whsec_") {
		return c, fmt.Errorf("Stripe credentials do not match STRIPE_MODE")
	}
	if raw := strings.TrimSpace(os.Getenv("STRIPE_API_BASE")); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return c, fmt.Errorf("invalid STRIPE_API_BASE")
		}
		local := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"
		if raw != "https://api.stripe.com" && (c.Mode != "test" || !local || (u.Scheme != "http" && u.Scheme != "https")) {
			return c, fmt.Errorf("custom Stripe API endpoints are restricted to loopback in test mode")
		}
		c.APIBase = strings.TrimRight(raw, "/")
	}
	c.PublicURL = (&config.Config{}).GetPublicURL()
	if cfg := config.GetGlobalConfig(); cfg != nil {
		c.PublicURL = cfg.GetPublicURL()
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(c.Mode == "test" && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return c, fmt.Errorf("Stripe requires a configured HTTPS PUBLIC_URL (loopback HTTP is allowed in test mode)")
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	c.SuccessURL = strings.TrimSpace(os.Getenv("STRIPE_SUCCESS_URL"))
	if c.SuccessURL != "" {
		s, err := url.Parse(c.SuccessURL)
		if err != nil || s.User != nil || s.Fragment != "" || !strings.EqualFold(s.Host, u.Host) || s.Scheme != u.Scheme {
			return c, fmt.Errorf("STRIPE_SUCCESS_URL must have the same origin as PUBLIC_URL")
		}
	}
	return c, nil
}

func (c stripeConfig) enabled() bool { return c.Key != "" && c.Secret != "" }

// ValidatePaymentConfiguration is also called at boot, before accepting traffic.
func (h *AdminHandler) ValidatePaymentConfiguration() error {
	c, err := loadStripeConfig()
	if err != nil {
		return err
	}
	if !c.enabled() {
		return h.repo.CheckPaymentMode(c.Mode)
	}
	return h.repo.BindPaymentMode(c.Mode)
}

func (c stripeConfig) returnURL(number, state string) string {
	raw := c.PublicURL + "/app/"
	if state == "success" && c.SuccessURL != "" {
		raw = c.SuccessURL
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	q.Set("tab", "wallet")
	q.Set("recharge", state)
	q.Set("order_no", number)
	u.RawQuery = q.Encode()
	return u.String()
}
