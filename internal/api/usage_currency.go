package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// normalizeUnbillableCurrency zeroes a computed charge whose pricing snapshot
// cannot settle in the wallet currency. The settlement path rejects such a
// record (and zeroes its audit row); the response header must agree with it.
func normalizeUnbillableCurrency(cost, savedCost float64, snapshot *storage.UsagePricingSnapshot) (float64, float64, bool) {
	if snapshot == nil {
		return cost, savedCost, false
	}
	currency := strings.TrimSpace(snapshot.Currency)
	if currency == "" || strings.EqualFold(currency, "CNY") {
		return cost, savedCost, false
	}
	return 0, 0, true
}

// The wallet settles only CNY. Reject a legacy foreign rate before work starts.
//
// C-14: the gate covers every model settlement may price, not only the requested
// one. When a channel maps the alias to a served model that has its own rate card,
// billing charges that rate card (see billingModelFor), so a non-CNY legacy row
// there would sail past a requested-model-only check and only surface as a zeroed
// charge after the upstream work was already paid for.
func validateModelCurrency(c *gin.Context, modelName string) bool {
	if billing.GlobalEngine == nil {
		return true
	}
	keyGroup := getKeyGroup(c)
	for _, candidate := range currencyModelsForBilling(modelName, keyGroup) {
		if err := storage.ValidatePriceCurrency(billing.GlobalEngine.GetPriceWithGroup(candidate, keyGroup)); err != nil {
			RespondOpenAIError(c, http.StatusUnprocessableEntity, err.Error(), "invalid_request_error", "unsupported_pricing_currency")
			return false
		}
	}
	return true
}
