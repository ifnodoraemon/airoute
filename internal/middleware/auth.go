package middleware

import (
	"context"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// authStorageTimeout bounds the per-request authoritative key/user lookups.
// The in-memory snapshot already carries the quota data; the database read is a
// consistency check, not something worth blocking a request (or the shared
// SQLite connection) for.
//
// Deliberately uncached: budget/quota changes written outside this process must
// be observed by the very next request, which a TTL cache would mask.
const authStorageTimeout = 2 * time.Second

const (
	ContextKeyTenant       = "tenant_id"
	ContextKeyAPIKey       = "api_key"
	ContextKeyAPIKeyConfig = "api_key_config"
	// ContextKeyAPIKeyID carries the key's public identifier (ak_…). Access logs
	// and audit lines name a key with it; the digest is never displayed.
	ContextKeyAPIKeyID = "api_key_id"
	// ContextKeySettledCost carries the amount actually charged for the request
	// once the billing layer has settled it. It is what lets the admission
	// middleware release the wallet pre-authorisation with the real number
	// instead of the coarse estimate it reserved at authentication time.
	ContextKeySettledCost = "airoute_settled_cost_cny"

	// The in-flight reservation is booked at authentication with a coarse amount,
	// then topped up at dispatch once the model, prompt and max_tokens are known.
	// These keys carry what the top-up needs so every handler can refine it
	// without re-reading the key or the wallet.
	ContextKeyQuotaUserID   = "airoute_quota_user_id"
	ContextKeyQuotaBackend  = "airoute_quota_backend"
	ContextKeyQuotaBalance  = "airoute_quota_balance_cny"
	ContextKeyQuotaReserved = "airoute_quota_reserved_cny"
)

// TopUpUserQuota raises this request's wallet pre-authorisation to at least
// targetCNY, clamped to what the wallet can still cover.
//
// Admission reserves a coarse amount before the body is parsed; this is called
// once the request is understood (model, prompt size, max_tokens), so the
// reserved amount is the conservative cost estimate rather than a constant. A
// request with no reservation (admin, free read, snapshot without user info) is
// a no-op, as is a target below what is already reserved.
//
// The release in AuthMiddleware reads the reserved total back from the context,
// so a successful top-up is always released at the increased amount.
func TopUpUserQuota(c *gin.Context, targetCNY float64) {
	if c == nil || !isFiniteNonNegative(targetCNY) {
		return
	}
	reservedValue, ok := c.Get(ContextKeyQuotaReserved)
	if !ok {
		return
	}
	current, _ := reservedValue.(float64)
	additional := targetCNY - current
	if additional <= 0 {
		return
	}
	userIDValue, _ := c.Get(ContextKeyQuotaUserID)
	userID, _ := userIDValue.(int64)
	backendValue, _ := c.Get(ContextKeyQuotaBackend)
	backend, _ := backendValue.(QuotaBackend)
	if userID <= 0 || backend == QuotaBackendNone {
		return
	}
	balanceValue, _ := c.Get(ContextKeyQuotaBalance)
	balance, _ := balanceValue.(float64)

	needUnits := reserveQuotaUnits(additional)
	if needUnits <= 0 {
		return
	}

	addedCNY := topUpReservation(userID, backend, balance, needUnits)
	if addedCNY <= 0 {
		return
	}
	c.Set(ContextKeyQuotaReserved, current+addedCNY)
}

// TopUpUserQuotaForUser is TopUpUserQuota for callers that hold the reservation
// identity directly instead of a gin context (the MCP transport). It returns the
// total amount now reserved for the request, so the matching release can pass it
// to ReleaseUserQuotaAmount.
func TopUpUserQuotaForUser(userID int64, backend QuotaBackend, balanceCNY, targetCNY, reservedCNY float64) float64 {
	if userID <= 0 || backend == QuotaBackendNone || !isFiniteNonNegative(targetCNY) {
		return reservedCNY
	}
	additional := targetCNY - reservedCNY
	if additional <= 0 {
		return reservedCNY
	}
	addedCNY := topUpReservation(userID, backend, balanceCNY, reserveQuotaUnits(additional))
	return reservedCNY + addedCNY
}

// topUpReservation adds one amount top-up on the backend that granted the
// reservation and returns the CNY actually added (0 when nothing was added).
func topUpReservation(userID int64, backend QuotaBackend, balanceCNY float64, needUnits int64) float64 {
	if userID <= 0 || needUnits <= 0 {
		return 0
	}
	switch backend {
	case QuotaBackendRedis:
		rClient := distributed.GetClient()
		if rClient == nil || !rClient.IsActive() {
			return 0
		}
		ctx, cancel := context.WithTimeout(context.Background(), inflightQuotaTimeout)
		added, err := rClient.TopUpQuotaAmount(ctx, userID, storage.ToUnits(balanceCNY), needUnits)
		cancel()
		if err != nil {
			telemetry.ScopedLogger("sync").Warn("redis in-flight quota top-up failed; keeping the admission-time reserve",
				"error", err.Error(), "user_id", userID)
			return 0
		}
		if added <= 0 {
			return 0
		}
		addReservedUnits(added)
		return float64(added) / float64(storage.UnitsPerCurrency)
	case QuotaBackendLocal:
		added := globalQuotaTracker.topUpLocal(userID, balanceCNY, needUnits)
		if added <= 0 {
			return 0
		}
		addReservedUnits(added)
		return float64(added) / float64(storage.UnitsPerCurrency)
	}
	return 0
}

// reservedQuotaCNY returns the amount currently reserved for this request,
// including any dispatch-time top-up.
func reservedQuotaCNY(c *gin.Context) float64 {
	if c == nil {
		return 0
	}
	value, ok := c.Get(ContextKeyQuotaReserved)
	if !ok {
		return 0
	}
	amount, _ := value.(float64)
	return amount
}

// shouldFailClosedOnUnknownAccount decides the admission posture when the key
// belongs to a user but the gateway cannot establish that user's status or
// balance (snapshot built without user fields + authoritative lookup timed out).
//
// The old behaviour was to admit: the request skipped the wallet gate and the
// pre-authorisation, which is exactly the burst the amount reservation exists to
// stop. Free reads are exempt — they never settle a charge, and blocking a
// job-status poll during a database stall protects no money.
func shouldFailClosedOnUnknownAccount(freeRead bool, userStatus string, userID int64) bool {
	return !freeRead && userStatus == "" && userID > 0
}

// SetSettledCost records the cost actually charged for the request, so the
// in-flight reservation taken in AuthMiddleware can be released against it.
//
// It is called by the billing/settlement layer after a charge succeeds. The
// reservation release happens in AuthMiddleware's deferred cleanup, which runs
// after every downstream handler, so a value set anywhere during the request is
// visible there. A negative or non-finite cost is ignored: the reservation must
// still be released, and a bogus "credit" must not be able to erase it.
func SetSettledCost(c *gin.Context, cost float64) {
	if c == nil || !isFiniteNonNegative(cost) {
		return
	}
	c.Set(ContextKeySettledCost, cost)
}

// SettledCost returns the cost recorded by SetSettledCost, if the request has
// one. Its second result distinguishes "settled at zero" (a free request) from
// "not settled".
func SettledCost(c *gin.Context) (float64, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.Get(ContextKeySettledCost)
	if !ok {
		return 0, false
	}
	cost, ok := value.(float64)
	if !ok || !isFiniteNonNegative(cost) {
		return 0, false
	}
	return cost, true
}

// settleReleaseAmount decides how much of the in-flight reservation to release
// for a request that reserved `reserve` CNY.
//
// The settled cost is preferred when it is larger than the reservation: an
// unplanned overrun must not silently stay credited as if it had not happened.
// It is never used when it is smaller, because the distributed counter holds the
// sum of reservations - releasing less than was booked would leak the difference
// against the wallet permanently (every request would leave 0.05-actual behind).
// Under-reserving is corrected at dispatch by topping the reservation up to the
// estimate, not by releasing a different amount at the end.
func settleReleaseAmount(c *gin.Context, reserve float64) float64 {
	if settled, ok := SettledCost(c); ok && settled > reserve {
		return settled
	}
	return reserve
}

func isFiniteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// auditAuthFailure logs a client-side admission rejection (insufficient balance,
// disabled or throttled key) into the audit table so 402/403/429 denials stay
// visible to operators.
//
// A 401 is deliberately not persisted. It means the caller never authenticated:
// no key matched, no request reached routing, nothing was billed. The row could
// only carry an empty model and channel, so it added noise to the request log and
// skewed the error rate. The access log still records the same trace id with its
// path, peer address and user agent.
//
// The opposite direction is unaffected: a 401/403 returned by an upstream
// provider is a channel failure on a real, billable request and is persisted
// with the upstream status text, because that means our own credential to the
// provider is broken.
func auditAuthFailure(c *gin.Context, rawKey string, statusCode int, errMsg string) {
	if statusCode == http.StatusUnauthorized {
		return
	}
	// Record the digest, exactly as a successful call does: usage_logs.api_key is
	// the join key back to api_keys, and a digest is not a usable credential. A
	// value that never matched a row is not recorded (the 401 early-return above
	// covers it), so no fragment of an unrecognised submission is persisted.
	auditDigest := ""
	if rawKey != "" {
		auditDigest = storage.HashAPIKey(rawKey)
	}
	recordAdmissionRejection(c, auditDigest, statusCode, errMsg)
}

// auditDenial records a denial raised after authentication has already resolved the
// credential: the request context holds the stored digest (ContextKeyAPIKey), so it
// is written as-is. Hashing it again would produce a row that joins to no key.
//
// Rate limiting runs behind AuthMiddleware, so routing its 429s here is what makes
// the auditAuthFailure contract ("402/403/429 stay visible") actually hold.
func auditDenial(c *gin.Context, denial *AccessDenial) {
	if denial == nil {
		return
	}
	auditDigest := ""
	if c != nil {
		auditDigest = c.GetString(ContextKeyAPIKey)
	}
	recordAdmissionRejection(c, auditDigest, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
}

// recordAdmissionRejection is the single writing funnel for admission rejections:
// every 402/403/429 exit goes through here, so a newly added denial path cannot
// silently skip the audit trail. 401 is filtered by auditAuthFailure.
func recordAdmissionRejection(c *gin.Context, auditDigest string, statusCode int, errMsg string) {
	if c == nil || storage.GlobalAsyncLogger == nil {
		return
	}
	traceID := c.GetString("trace_id")
	if traceID == "" {
		traceID = c.GetHeader("x-request-id")
	}
	storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
		TraceID:      traceID,
		APIKey:       auditDigest,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		UsageSource:  "gateway_auth",
	})
}

// dataPlaneReady distinguishes "the gateway genuinely has no API keys" from
// "the runtime configuration has not been loaded yet".
//
// It defaults to true so embedders and tests that never wire the startup path
// keep working; cmd/airoute sets it to false before the first snapshot load and
// flips it back once storage answered. Without that distinction a failed first
// snapshot read leaves the in-memory key set empty, and the open-dev-mode
// branch below would admit every caller anonymously.
var dataPlaneReady atomic.Bool

func init() { dataPlaneReady.Store(true) }

// SetDataPlaneReady marks whether the runtime configuration has been loaded
// from durable storage at least once. It is called by the server bootstrap.
func SetDataPlaneReady(ready bool) { dataPlaneReady.Store(ready) }

// DataPlaneReady reports whether the data plane has a loaded configuration.
func DataPlaneReady() bool { return dataPlaneReady.Load() }

// DataPlaneUnreadyDenial is the fail-closed response served while the gateway
// has not loaded its configuration yet.
func DataPlaneUnreadyDenial() *AccessDenial {
	return &AccessDenial{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "Gateway data plane is not ready: configuration has not been loaded from storage yet. Please retry shortly.",
		Type:       "service_unavailable",
		Code:       "data_plane_unready",
		Headers:    map[string]string{"Retry-After": "5"},
		LogMessage: "data plane configuration not loaded yet",
	}
}

// AuthMiddleware authenticates incoming requests via Bearer API keys.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetGlobalConfig()

		// Fail closed until the first successful configuration load. The open-mode
		// check below must never be reached with a snapshot that was never read.
		if !DataPlaneReady() {
			denial := DataPlaneUnreadyDenial()
			auditAuthFailure(c, "", denial.StatusCode, denial.LogMessage)
			for header, value := range denial.Headers {
				c.Header(header, value)
			}
			AbortWithDenial(c, denial)
			return
		}

		// If no API keys configured in system and no keys ever created, allow all requests in open dev mode.
		// If keys have been registered in the gateway (even if currently disabled/revoked), strictly enforce auth!
		// Both fields are rewritten by the periodic hot reload, so they must be
		// read through the locked accessors (see config.ActiveAPIKeyCount).
		if cfg.ActiveAPIKeyCount() == 0 && !cfg.HasRegisteredAPIKeys() {
			c.Next()
			return
		}

		var rawKey string

		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				rawKey = strings.TrimSpace(parts[1])
			}
		}

		// Also support Anthropic SDK's x-api-key header
		if rawKey == "" {
			rawKey = strings.TrimSpace(c.GetHeader("x-api-key"))
		}

		// Also support Google Gemini SDK's query param 'key' and header 'x-goog-api-key'
		if rawKey == "" {
			rawKey = strings.TrimSpace(c.Query("key"))
		}
		if rawKey == "" {
			rawKey = strings.TrimSpace(c.GetHeader("x-goog-api-key"))
		}

		if rawKey == "" {
			auditAuthFailure(c, "", http.StatusUnauthorized, "Missing authentication key")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing authentication key. Please provide 'Authorization: Bearer <key>', 'x-api-key: <key>', 'x-goog-api-key: <key>', or query parameter '?key=<key>'",
					"type":    "invalid_request_error",
					"code":    "missing_api_key",
				},
			})
			return
		}
		// The snapshot is keyed by the stored digest (api_keys.key holds
		// HashAPIKey(credential)), so the presented credential is hashed before
		// the lookup. Hashing is deterministic, which keeps every existing
		// credential authenticating after the migration.
		keyHash := storage.HashAPIKey(rawKey)
		matchedKey := lookupSnapshotKey(cfg, rawKey, keyHash)

		// The snapshot is a cache with a synchronisation lag, so a miss is not proof
		// that the credential is invalid: a key created on another replica, or one
		// revoked since the last reload, exists only in durable storage at this
		// instant. Ask storage once before rejecting, and when it does know the
		// credential report its real state (revoked/disabled) instead of the generic
		// invalid-key message.
		var effectiveKey model.APIKeyConfig
		resolvedFromStorage := false
		if matchedKey != nil {
			effectiveKey = *matchedKey
		} else {
			keyRec := lookupStorageCredential(c, rawKey, keyHash)
			if keyRec == nil {
				auditAuthFailure(c, rawKey, http.StatusUnauthorized, "Incorrect or unauthorized API key provided")
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": gin.H{
						"message": "Incorrect or unauthorized API key provided.",
						"type":    "authentication_error",
						"code":    "invalid_api_key",
					},
				})
				return
			}
			if denial := KeyStatusDenial(keyRec.Status); denial != nil {
				auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
				AbortWithDenial(c, denial)
				return
			}
			effectiveKey = keyRec.ToConfig()
			resolvedFromStorage = true
		}

		// A GET video-task query is a read of a job that was already billed when
		// it was created. It must stay reachable after the wallet/budget is
		// exhausted, so the admission checks below skip only the spending gates
		// (balance, per-key budget, in-flight reservation) for this exact path.
		freeRead := IsFreeReadVideoTaskQuery(c)

		// 1. Key status check (pure in-memory)
		if denial := KeyStatusDenial(effectiveKey.Status); denial != nil {
			auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
			AbortWithDenial(c, denial)
			return
		}

		// 2. Key budget check (fast in-memory check first). Use the integer
		// 1e-9-unit mirror when available so repeated float accumulation cannot
		// drift across the budget boundary.
		if !freeRead {
			if denial := KeyBudgetDenial(effectiveKey.BudgetUnits, effectiveKey.UsedCostUnits, effectiveKey.Budget, effectiveKey.UsedCost); denial != nil {
				auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
				AbortWithDenial(c, denial)
				return
			}
		}

		// 3. Authoritative storage key check if database repository is available.
		// Bounded by a short deadline: this runs on every data-plane request, so a
		// stalled database must fail the request rather than pin the goroutine
		// (and, on SQLite's single connection, the whole gateway).
		//
		// Skipped when the credential was just resolved from storage in the fallback
		// above: the row in hand is already the authoritative, newest state, and a
		// second query would only duplicate the work.
		repo := storage.GetGlobalRepository()
		if repo != nil && !resolvedFromStorage {
			lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
			keyRec, _ := repo.GetAPIKeyByKeyContext(lookupCtx, effectiveKey.Key)
			cancel()
			if keyRec != nil {
				if denial := KeyStatusDenial(keyRec.Status); denial != nil {
					auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
					AbortWithDenial(c, denial)
					return
				}
				effectiveBudget := keyRec.Budget
				if effectiveBudget <= 0 {
					effectiveBudget = effectiveKey.Budget
				}
				budgetUnits := keyRec.BudgetUnits
				if budgetUnits <= 0 {
					budgetUnits = effectiveKey.BudgetUnits
				}
				if denial := KeyBudgetDenial(budgetUnits, keyRec.UsedCostUnits, effectiveBudget, keyRec.UsedCost); denial != nil && !freeRead {
					auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
					AbortWithDenial(c, denial)
					return
				}
				if keyRec.GroupName != "" {
					effectiveKey.GroupName = keyRec.GroupName
				}
				if keyRec.UserID > 0 {
					effectiveKey.UserID = keyRec.UserID
				}
				if keyRec.ID > 0 {
					effectiveKey.ID = keyRec.ID
				}
				if keyRec.UserStatus != "" {
					effectiveKey.UserStatus = keyRec.UserStatus
					effectiveKey.UserRole = keyRec.UserRole
					effectiveKey.UserBalance = keyRec.UserBalance
				}
			}
		}

		// 4. User account status & wallet balance check
		userStatus := effectiveKey.UserStatus
		userRole := effectiveKey.UserRole
		userBalance := effectiveKey.UserBalance
		userID := effectiveKey.UserID

		// Fallback to storage only if memory snapshot was not populated
		if userStatus == "" && effectiveKey.UserID > 0 && repo != nil {
			lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
			u, _ := repo.GetUserByIDContext(lookupCtx, effectiveKey.UserID)
			cancel()
			if u != nil {
				userStatus = u.Status
				userRole = u.Role
				userBalance = u.Balance
			}
		}

		// Fail closed when the key has an owner whose state we cannot establish.
		// This only happens when the snapshot lacks the user fields and the
		// authoritative lookup timed out; admitting the request would skip both the
		// balance gate and the pre-authorisation. Free reads are exempt: they never
		// settle a charge, and blocking a job-status poll during a DB stall would
		// not protect any money.
		if shouldFailClosedOnUnknownAccount(freeRead, userStatus, userID) {
			denial := BalanceUnknownDenial()
			auditAuthFailure(c, rawKey, denial.StatusCode, denial.LogMessage)
			AbortWithDenial(c, denial)
			return
		}

		if freeRead {
			// Locked/deleted accounts stay blocked; only the wallet balance gate
			// is waived for a read that never bills.
			if denial := AccountLockedDenial(userStatus); denial != nil {
				auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
				AbortWithDenial(c, denial)
				return
			}
		} else if denial := AccountDenial(userStatus, userRole, userBalance); denial != nil {
			auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
			AbortWithDenial(c, denial)
			return
		}

		// Pre-flight amount pre-authorisation: the wallet must cover every amount
		// already reserved for requests in flight plus this request's reserve, and
		// the user may not exceed the in-flight concurrency cap. A free read
		// reserves nothing because it will never settle a charge.
		if !freeRead && userStatus != "" && !strings.EqualFold(userRole, "admin") && userID > 0 {
			reserve := InflightReserveCNY()
			acquired, backend := ReserveUserQuotaAmount(userID, userBalance, reserve)
			if !acquired {
				denial := &AccessDenial{
					StatusCode: http.StatusPaymentRequired,
					Message:    "Concurrent requests exceed available wallet balance reserve. Please wait for pending requests to finish or recharge.",
					Type:       "insufficient_quota",
					Code:       "insufficient_quota",
				}
				auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
				AbortWithDenial(c, denial)
				return
			}
			c.Set(ContextKeyQuotaUserID, userID)
			c.Set(ContextKeyQuotaBackend, backend)
			c.Set(ContextKeyQuotaBalance, userBalance)
			c.Set(ContextKeyQuotaReserved, reserve)
			// A Redis reservation expires on a short TTL, so a request that runs
			// longer than that TTL has to keep refreshing it (see
			// StartQuotaHeartbeat); the local tracker does not need it.
			stopHeartbeat := StartQuotaHeartbeat(userID, backend)
			// Release after the request is settled: the billing layer reports the
			// real cost through SetSettledCost, and a dispatch-time top-up may have
			// raised the reservation. Releasing less than was booked would leak the
			// difference against the wallet permanently.
			//
			// Stop the heartbeat first and synchronously: a refresh racing with the
			// release would re-create the TTL on keys that are about to be deleted,
			// or extend a later request's reservation.
			defer func() {
				stopHeartbeat()
				ReleaseUserQuotaAmount(userID, backend, settleReleaseAmount(c, reservedQuotaCNY(c)))
			}()
		}

		// Save context info
		c.Set(ContextKeyTenant, effectiveKey.TenantID)
		// The digest is what audit rows and ledger joins use; the public id is
		// what logs and the console display.
		c.Set(ContextKeyAPIKey, effectiveKey.Key)
		c.Set(ContextKeyAPIKeyID, effectiveKey.PublicID)
		c.Set(ContextKeyAPIKeyConfig, &effectiveKey)
		c.Next()
	}
}

// ValidateModelAllowed checks if the requested model is permitted for this API key.
func ValidateModelAllowed(c *gin.Context, requestedModel string) bool {
	keyAny, exists := c.Get(ContextKeyAPIKeyConfig)
	if !exists {
		return true // open mode
	}

	keyCfg, ok := keyAny.(*model.APIKeyConfig)
	if !ok {
		return true // all models allowed
	}
	return ModelAllowed(keyCfg.AllowedModels, requestedModel)
}

// budgetExceeded compares spend against a budget using integer nano-units when
// the ledger mirror is populated, falling back to the float columns for legacy
// rows.
//
// The previous condition required BOTH sides to be positive, so a key configured
// with only budget_nano (float budget = 0) or a key that had not spent anything
// yet bypassed the integer check entirely and could be unbounded. The integer
// mirror is now authoritative whenever a budget is set in it.
func budgetExceeded(budgetUnits, usedUnits int64, budget, used float64) bool {
	if budgetUnits > 0 {
		return usedUnits >= budgetUnits
	}
	return budget > 0 && used >= budget
}

// lookupSnapshotKey resolves a presented credential against the in-memory snapshot.
//
// The snapshot is keyed by the stored digest. A snapshot built before migration 29
// (or seeded directly by a test) still holds cleartext identifiers; in that case the
// raw value is tried as well, which is exactly the pre-migration behaviour.
//
// The raw fallback is deliberately limited to values that cannot themselves be a
// digest: otherwise the stored digest — which is not a credential — would become
// replayable as a bearer token.
func lookupSnapshotKey(cfg *config.Config, presented, presentedHash string) *model.APIKeyConfig {
	if cfg == nil {
		return nil
	}
	if matched := cfg.GetAPIKey(presentedHash); matched != nil {
		return matched
	}
	if presented == "" || storage.IsAPIKeyHash(presented) {
		return nil
	}
	legacy := cfg.GetAPIKey(presented)
	if legacy == nil {
		return nil
	}
	// Hand downstream code the digest rather than the legacy identifier: the
	// authoritative database check, audit rows and ledger joins all key off it.
	normalized := *legacy
	normalized.Key = presentedHash
	return &normalized
}
