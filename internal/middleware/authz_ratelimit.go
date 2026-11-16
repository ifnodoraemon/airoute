package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// CheckKeyRateLimits applies the per-key TPM and RPM limits. HTTP requests reach
// it through RateLimitMiddleware; the MCP transport calls it directly, because
// /mcp/* was never mounted behind the rate-limit middleware.
func CheckKeyRateLimits(ctx context.Context, key *model.APIKeyConfig) KeyRateLimitOutcome {
	outcome := KeyRateLimitOutcome{RemainingTPM: -1}
	if key == nil {
		return outcome
	}

	// Token-per-minute gate: reject before any upstream call when the key has
	// already consumed its minute budget. RPM limiting below stays independent.
	if key.TPM > 0 {
		if !GlobalTokenQuota.Allow(key.Key, key.TPM) {
			outcome.Denial = &AccessDenial{
				StatusCode: http.StatusTooManyRequests,
				Message:    "Token rate limit exceeded (TPM limit reached). Please slow down requests.",
				Type:       "rate_limit_error",
				Code:       "token_rate_limit_exceeded",
				Headers: map[string]string{
					"X-RateLimit-Limit-Tokens": strconv.Itoa(key.TPM),
					"Retry-After":              "60",
				},
				LogMessage: "token rate limit exceeded (TPM)",
			}
			return outcome
		}
		outcome.RemainingTPM = GlobalTokenQuota.Remaining(key.Key, key.TPM)
	}

	if key.RPM <= 0 {
		return outcome
	}

	// 1. Enterprise distributed Redis cluster rate limit
	client := distributed.GetClient()
	if client != nil && client.IsActive() {
		rpmCtx, cancel := context.WithTimeout(ctx, redisRateLimitTimeout)
		allowed, err := client.AllowRPM(rpmCtx, key.Key, key.RPM)
		cancel()
		if err == nil {
			if !allowed {
				outcome.Denial = &AccessDenial{
					StatusCode: http.StatusTooManyRequests,
					Message:    "Cluster rate limit exceeded (RPM limit reached). Please slow down requests.",
					Type:       "rate_limit_error",
					Code:       "rate_limit_exceeded",
					Headers: map[string]string{
						"X-RateLimit-Limit-Requests": strconv.Itoa(key.RPM),
						"Retry-After":                "1",
					},
					LogMessage: "cluster rate limit exceeded (RPM)",
				}
			}
			return outcome
		}
		// On Redis error/timeout, gracefully fall through to the local bucket.
	}

	// 2. Fallback to the local in-memory sliding window. It enforces the same 60s
	// semantics as the Redis path above, so a Redis outage does not loosen the
	// limit (see rpmWindowSpan).
	bucket := GlobalRateLimiter.GetBucket(key.Key, key.RPM)
	if !bucket.Allow() {
		outcome.Denial = &AccessDenial{
			StatusCode: http.StatusTooManyRequests,
			Message:    "Rate limit exceeded (RPM limit reached). Please slow down requests.",
			Type:       "rate_limit_error",
			Code:       "rate_limit_exceeded",
			Headers: map[string]string{
				"X-RateLimit-Limit-Requests": strconv.Itoa(key.RPM),
				"Retry-After":                "1",
			},
			LogMessage: "local rate limit exceeded (RPM)",
		}
	}
	return outcome
}