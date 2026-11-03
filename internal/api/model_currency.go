package api

import (
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// servedModelsForModel enumerates the upstream model names that the channels
// serving modelName would actually call (their model mapping applied). It is
// installed once at startup because the dispatcher is owned by the server, not by
// this package, and validateModelCurrency is a free function used by every
// ingress handler.
var servedModelsForModel func(modelName string) []string

// installServedModelEnumerator wires the read-only channel mapping used by the
// pricing-currency pre-flight gate. It performs no selection (no shared selector
// state is mutated) and is idempotent, so tests that build a router repeatedly
// converge on the last dispatcher.
func installServedModelEnumerator(d *router.Dispatcher) {
	if d == nil {
		servedModelsForModel = nil
		return
	}
	servedModelsForModel = func(modelName string) []string {
		channels := d.ListChannelsForModel(modelName, "")
		if len(channels) == 0 {
			return nil
		}
		served := make([]string, 0, len(channels))
		for _, ch := range channels {
			mapped := ch.GetUpstreamModel(modelName)
			if mapped == "" || mapped == modelName {
				continue
			}
			served = append(served, mapped)
		}
		return served
	}
}

// currencyModelsForBilling lists every model whose rate card settlement may price
// for a request to modelName: the requested model itself, plus the served model of
// any candidate channel whose mapped name is priced explicitly.
//
// The second half mirrors billingModelFor exactly: billing follows the channel's
// mapping only when the served model has a configured price, so that is the only
// case where its rate card (and therefore its currency) can be used. Checking it
// up front turns "request 200 with cost 0 after the upstream was already paid for"
// into the documented 422 before any upstream work starts.
func currencyModelsForBilling(modelName, keyGroup string) []string {
	models := []string{modelName}
	if servedModelsForModel == nil || billing.GlobalEngine == nil {
		return models
	}
	seen := map[string]struct{}{modelName: {}}
	for _, served := range servedModelsForModel(modelName) {
		if served == "" {
			continue
		}
		if _, ok := seen[served]; ok {
			continue
		}
		if billing.GlobalEngine.ResolvePriceSource(served, keyGroup) == billing.PriceSourceDefault {
			continue
		}
		seen[served] = struct{}{}
		models = append(models, served)
	}
	return models
}
