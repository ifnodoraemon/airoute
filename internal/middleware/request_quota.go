package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const contextKeyRequestQuota = "airoute_request_quota"

// Reading an already-paid result or cancelling a held job cannot require a
// second reservation. Account/key status and resource ownership still apply.
func isUnbilledResourceRequest(c *gin.Context) bool {
	if IsFreeReadVideoTaskQuery(c) {
		return true
	}
	if c.Request.Method == http.MethodDelete {
		return true
	}
	path := c.Request.URL.Path
	for _, prefix := range []string{"/v1beta/batches/", "/v1beta/operations/", "/v1beta/interactions/", "/v1/interactions/", "/v1/responses/"} {
		if strings.HasPrefix(path, prefix) {
			return c.Request.Method == http.MethodGet || (c.Request.Method == http.MethodPost && strings.HasSuffix(path, ":cancel"))
		}
	}
	return false
}

// RequestQuota belongs to one handler goroutine. The database, rather than a
// process-local counter or Redis TTL, owns the cross-replica reservation.
type RequestQuota struct {
	repo        *storage.Repository
	Reservation *storage.BillingReservation
	pending     bool
}

func AcquireRequestQuota(ctx context.Context, repo *storage.Repository, key string) (*RequestQuota, error) {
	if repo == nil {
		return nil, storage.ErrReservationAccount
	}
	ctx, cancel := context.WithTimeout(ctx, authStorageTimeout)
	defer cancel()
	rec, err := repo.ReserveBilling(ctx, key, reserveQuotaUnits(InflightReserveCNY()), maxInflightPerUser)
	if err != nil || rec == nil {
		return nil, err
	}
	return &RequestQuota{repo: repo, Reservation: rec}, nil
}

func (q *RequestQuota) Resize(ctx context.Context, targetCNY float64) error {
	if q == nil {
		return nil
	}
	if !isFiniteNonNegative(targetCNY) {
		return storage.ErrWalletReserved
	}
	units := reserveQuotaUnits(targetCNY)
	if units <= q.Reservation.AmountUnits {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, authStorageTimeout)
	defer cancel()
	if err := q.repo.ResizeBilling(ctx, q.Reservation.ID, units); err != nil {
		return err
	}
	q.Reservation.AmountUnits = units
	return nil
}

func (q *RequestQuota) Close() {
	if q == nil || q.pending {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), authStorageTimeout)
	defer cancel()
	if err := q.repo.CancelBilling(ctx, q.Reservation.ID); err != nil {
		telemetry.ScopedLogger("billing").Error("billing reservation cancellation deferred", "reservation_id", q.Reservation.ID, "error", err.Error())
	}
}

// Hold transfers the request's reservation to an asynchronous provider job.
// Its ID is saved with the job and released by that job's final settlement.
func (q *RequestQuota) Hold(ctx context.Context) error {
	if q == nil {
		return nil
	}
	q.pending = true
	return q.repo.PendBilling(ctx, q.Reservation.ID, q.Reservation.ID, q.Reservation.AmountUnits)
}

// Pend runs before handing the charge to either writer. Once service has been
// delivered, a timeout or panic must retain the hold until settlement commits.
func (q *RequestQuota) Pend(rec *storage.UsageLogRecord) {
	if q == nil || rec == nil || rec.Cost <= 0 {
		return
	}
	q.pending = true
	rec.ReservationID = q.Reservation.ID
	if rec.BillingID == "" {
		rec.BillingID = q.Reservation.ID
	}
	ctx, cancel := context.WithTimeout(context.Background(), authStorageTimeout)
	defer cancel()
	if err := q.repo.PendBillingRecord(ctx, rec); err != nil {
		telemetry.ScopedLogger("billing").Error("billing reservation remains held awaiting settlement", "reservation_id", rec.ReservationID, "error", err.Error())
	}
}

func RequestQuotaFrom(c *gin.Context) *RequestQuota {
	if c == nil {
		return nil
	}
	value, _ := c.Get(contextKeyRequestQuota)
	quota, _ := value.(*RequestQuota)
	return quota
}

func RequireRequestQuota(c *gin.Context, targetCNY float64) bool {
	quota := RequestQuotaFrom(c)
	if quota == nil {
		return true
	}
	if err := quota.Resize(c.Request.Context(), targetCNY); err != nil {
		AbortWithDenial(c, QuotaErrorDenial(err))
		return false
	}
	return true
}

func QuotaErrorDenial(err error) *AccessDenial {
	switch {
	case errors.Is(err, storage.ErrWalletReserved):
		return &AccessDenial{StatusCode: http.StatusPaymentRequired, Message: "Available wallet balance cannot cover this request and its pending charges.", Type: "insufficient_quota", Code: "insufficient_quota"}
	case errors.Is(err, storage.ErrKeyBudgetReserved):
		return &AccessDenial{StatusCode: http.StatusPaymentRequired, Message: "Available API key budget cannot cover this request and its pending charges.", Type: "key_budget_exceeded", Code: "key_budget_exceeded"}
	default:
		return &AccessDenial{StatusCode: http.StatusServiceUnavailable, Message: "Billing reservation is temporarily unavailable. Please retry shortly.", Type: "service_unavailable", Code: "billing_unavailable", Headers: map[string]string{"Retry-After": "2"}}
	}
}
