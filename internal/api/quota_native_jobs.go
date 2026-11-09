package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func requireFlatFeeAgentQuota(c *gin.Context, name string) bool {
	if billing.GlobalEngine == nil {
		RespondGeminiError(c, 503, "Billing prices are unavailable", "UNAVAILABLE")
		return false
	}
	group := getKeyGroup(c)
	for _, candidate := range currencyModelsForBilling(name, group) {
		price := billing.GlobalEngine.GetPriceWithGroup(candidate, group)
		if price == nil || price.FixedPrice <= 0 || price.PromptPrice != 0 || price.CompletionPrice != 0 || price.CacheReadPrice != 0 {
			RespondGeminiError(c, 422, "Prepaid agent interactions require fixed-price-only billing; variable token usage cannot be bounded", "INVALID_ARGUMENT")
			return false
		}
	}
	return requireQuotaEstimate(c, name, 0, 0, 1)
}

func reserveGoogleBatch(c *gin.Context, name, action string, body *[]byte) bool {
	if middleware.RequestQuotaFrom(c) == nil {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(provider.CanonicalGeminiJSON(*body)))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		RespondGeminiError(c, 400, "Invalid batch JSON", "INVALID_ARGUMENT")
		return false
	}
	prompt, output, count, err := boundBatchRequests(name, action, document)
	if err != nil || count == 0 {
		RespondGeminiError(c, 422, "Prepaid batches require inline requests with bounded output; file-backed input cannot be pre-authorized", "INVALID_ARGUMENT")
		return false
	}
	if !requireQuotaEstimate(c, name, prompt, output, count) {
		return false
	}
	*body, err = json.Marshal(document)
	return err == nil
}

func boundBatchRequests(name, action string, value any) (prompt, output, count int, err error) {
	switch node := value.(type) {
	case []any:
		for _, child := range node {
			p, o, n, e := boundBatchRequests(name, action, child)
			if e != nil {
				return 0, 0, 0, e
			}
			if p > math.MaxInt-prompt || o > math.MaxInt-output {
				return 0, 0, 0, fmt.Errorf("batch token count overflow")
			}
			prompt, output, count = prompt+p, output+o, count+n
		}
	case map[string]any:
		_, generate := node["contents"]
		_, embed := node["content"]
		if generate || (embed && (action == "batchEmbedContents" || action == "asyncBatchEmbedContent")) {
			wire, _ := json.Marshal(node)
			if generate {
				var input provider.GeminiRequest
				if e := json.Unmarshal(wire, &input); e != nil {
					return 0, 0, 0, e
				}
				req := convertInboundGeminiToCanonical(name, &input, false)
				req.Native = &model.NativeRequest{Protocol: model.NativeGemini, Body: wire}
				if e := boundPrepaidCompletion(req); e != nil {
					return 0, 0, 0, e
				}
				var bounded map[string]any
				decoder := json.NewDecoder(bytes.NewReader(req.Native.Body))
				decoder.UseNumber()
				_ = decoder.Decode(&bounded)
				for key, v := range bounded {
					node[key] = v
				}
				out, e := prepaidOutputTokens(req)
				return req.EstimatedPromptTokens(), out, 1, e
			}
			// Byte length is a conservative text-token bound, including structure.
			return len(wire), 0, 1, nil
		}
		for _, child := range node {
			p, o, n, e := boundBatchRequests(name, action, child)
			if e != nil {
				return 0, 0, 0, e
			}
			if p > math.MaxInt-prompt || o > math.MaxInt-output {
				return 0, 0, 0, fmt.Errorf("batch token count overflow")
			}
			prompt, output, count = prompt+p, output+o, count+n
		}
	}
	if prompt < 0 || output < 0 {
		return 0, 0, 0, fmt.Errorf("batch token count overflow")
	}
	return
}

func retainGoogleQuota(c *gin.Context, r *googleResource, id string, snapshot map[string]json.RawMessage) error {
	if r.state != nil && r.state.ID == id {
		return nil
	}
	quota := middleware.RequestQuotaFrom(c)
	if quota == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
	defer cancel()
	if err := quota.Hold(ctx); err != nil {
		return err
	}
	snapshot["_airoute_reservation_id"], _ = json.Marshal(quota.Reservation.ID)
	return nil
}

func googleUsageParams(c *gin.Context, r *googleResource) AuditRecordParams {
	params := AuditRecordParams{}
	if r.state == nil {
		return params
	}
	var key model.APIKeyConfig
	var extra struct {
		ReservationID string `json:"_airoute_reservation_id"`
	}
	_ = json.Unmarshal(r.state.Billing, &key)
	_ = json.Unmarshal(r.state.Billing, &extra)
	params.ReservationID = extra.ReservationID
	params.APIKey, params.TenantID, params.KeyGroup, params.KeyConfig = key.Key, key.TenantID, key.GroupName, &key
	return params
}
