package config

import (
	"os"
	"strconv"
	"strings"
)

// GetAdminUsername returns configured admin username, prioritizing env over config.
func (c *Config) GetAdminUsername() string {
	if u := os.Getenv("GATEWAY_ADMIN_USER"); u != "" {
		return strings.TrimSpace(u)
	}
	if u := os.Getenv("GATEWAY_ADMIN_USERNAME"); u != "" {
		return strings.TrimSpace(u)
	}
	if c != nil && c.Admin.Username != "" {
		return strings.TrimSpace(c.Admin.Username)
	}
	return "admin"
}

// GetAdminPassword returns configured admin password, prioritizing env over config.
func (c *Config) GetAdminPassword() string {
	if p := os.Getenv("GATEWAY_ADMIN_PASSWORD"); p != "" {
		return p
	}
	if c != nil && c.Admin.Password != "" {
		return c.Admin.Password
	}
	return "admin123"
}

// IsRegistrationAllowed checks if public self-registration is enabled.
func (c *Config) IsRegistrationAllowed() bool {
	if v := os.Getenv("ALLOW_REGISTRATION"); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}
	if c != nil && c.Auth.AllowRegistration != nil {
		return *c.Auth.AllowRegistration
	}
	return true
}

// IsEmailVerificationRequired checks if email verification code is required during registration.
func (c *Config) IsEmailVerificationRequired() bool {
	if v := os.Getenv("REQUIRE_EMAIL_VERIFICATION"); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}
	if c != nil && c.Auth.RequireEmailVerification != nil {
		return *c.Auth.RequireEmailVerification
	}
	return strings.TrimSpace(os.Getenv("SMTP_HOST")) != ""
}

// GetInitialUserBalance returns the initial balance for newly registered users.
func (c *Config) GetInitialUserBalance() float64 {
	if v := os.Getenv("INITIAL_USER_BALANCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	if v := os.Getenv("DEFAULT_TRIAL_BALANCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	if c != nil && c.Auth.InitialUserBalance >= 0 {
		return c.Auth.InitialUserBalance
	}
	return 5.0
}

// GetTokenExpiryHours returns JWT token expiration duration in hours.
func (c *Config) GetTokenExpiryHours() int {
	if v := os.Getenv("TOKEN_EXPIRY_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if c != nil && c.Auth.TokenExpiryHours > 0 {
		return c.Auth.TokenExpiryHours
	}
	return 168
}

// GetPublicURL returns configured public-facing gateway URL, prioritizing env over config.
func (c *Config) GetPublicURL() string {
	if u := os.Getenv("PUBLIC_URL"); u != "" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
		return strings.TrimRight(strings.TrimSpace(u), "/")
	}
	if u := os.Getenv("GATEWAY_PUBLIC_URL"); u != "" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
		return strings.TrimRight(strings.TrimSpace(u), "/")
	}
	if c != nil && c.Server.PublicURL != "" && (strings.HasPrefix(c.Server.PublicURL, "http://") || strings.HasPrefix(c.Server.PublicURL, "https://")) {
		return strings.TrimRight(strings.TrimSpace(c.Server.PublicURL), "/")
	}
	return ""
}
