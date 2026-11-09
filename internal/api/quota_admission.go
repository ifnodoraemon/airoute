package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

const defaultPrepaidCompletionTokens = 4096

func topUpQuotaForRequest(c *gin.Context, req *model.ChatCompletionRequest) bool {
	if req == nil || middleware.RequestQuotaFrom(c) == nil {
		return true
	}
	if billing.GlobalEngine == nil {
		RespondOpenAIError(c, http.StatusServiceUnavailable, "Billing prices are unavailable", "service_unavailable", "billing_unavailable")
		return false
	}
	if err := boundPrepaidCompletion(req); err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Unable to bound request output tokens", "invalid_request_error", "invalid_max_tokens")
		return false
	}
	output, err := prepaidOutputTokens(req)
	if err != nil {
		RespondOpenAIError(c, http.StatusBadRequest, "Requested output token count is too large", "invalid_request_error", "invalid_max_tokens")
		return false
	}
	return requireQuotaEstimate(c, req.Model, req.EstimatedPromptTokens(), output, 1)
}

func prepaidOutputTokens(req *model.ChatCompletionRequest) (int, error) {
	count := 1
	if req.N != nil {
		count = max(1, *req.N)
	}
	if req.Native != nil && req.Native.Protocol == model.NativeGemini {
		var body struct {
			Config struct {
				Count *int `json:"candidateCount"`
			} `json:"generationConfig"`
		}
		if err := json.Unmarshal(req.Native.Body, &body); err != nil {
			return 0, err
		}
		if body.Config.Count != nil {
			count = max(1, *body.Config.Count)
		}
	}
	if *req.MaxTokens > math.MaxInt/count {
		return 0, fmt.Errorf("output token count overflow")
	}
	return *req.MaxTokens * count, nil
}

func requireQuotaEstimate(c *gin.Context, name string, prompt, completion, units int) bool {
	if middleware.RequestQuotaFrom(c) == nil {
		return true
	}
	if billing.GlobalEngine == nil {
		RespondOpenAIError(c, http.StatusServiceUnavailable, "Billing prices are unavailable", "service_unavailable", "billing_unavailable")
		return false
	}
	return middleware.RequireRequestQuota(c, estimateQuotaCost(name, getKeyGroup(c), prompt, completion, units))
}

func estimateQuotaCost(name, group string, prompt, completion, units int) float64 {
	cost := 0.0
	for _, candidate := range currencyModelsForBilling(name, group) {
		estimate := billing.EstimateCostCNY(candidate, group, prompt, completion) + billing.EstimateFixedCostCNY(candidate, group, units)
		cost = max(cost, estimate)
	}
	return cost
}

// A missing generation cap is bounded on the wire as well as in the estimate.
// Native protocols retain all other fields, including provider extensions.
func boundPrepaidCompletion(req *model.ChatCompletionRequest) error {
	if req.Native == nil {
		if req.MaxTokens == nil || *req.MaxTokens <= 0 {
			cap := defaultPrepaidCompletionTokens
			req.MaxTokens = &cap
		}
		return nil
	}
	body := map[string]json.RawMessage{}
	if err := json.Unmarshal(req.Native.Body, &body); err != nil {
		return err
	}
	field := "max_tokens"
	switch req.Native.Protocol {
	case model.NativeChat:
		if _, exists := body["max_completion_tokens"]; exists {
			field = "max_completion_tokens"
		}
	case model.NativeResponses:
		field = "max_output_tokens"
	case model.NativeGemini:
		return boundNestedCompletion(req, body, "generationConfig", "maxOutputTokens")
	case model.NativeGeminiInteractions:
		return boundNestedCompletion(req, body, "generation_config", "max_output_tokens")
	}
	if err := readCompletionCap(req, body[field]); err != nil {
		return err
	}
	body[field], _ = json.Marshal(*req.MaxTokens)
	encoded, err := json.Marshal(body)
	if err == nil {
		copy := *req.Native
		copy.Body = encoded
		req.Native = &copy
	}
	return err
}

func boundNestedCompletion(req *model.ChatCompletionRequest, body map[string]json.RawMessage, parent, field string) error {
	nested := map[string]json.RawMessage{}
	if len(body[parent]) > 0 {
		if err := json.Unmarshal(body[parent], &nested); err != nil {
			return err
		}
	}
	if err := readCompletionCap(req, nested[field]); err != nil {
		return err
	}
	nested[field], _ = json.Marshal(*req.MaxTokens)
	body[parent], _ = json.Marshal(nested)
	encoded, err := json.Marshal(body)
	if err == nil {
		copy := *req.Native
		copy.Body = encoded
		req.Native = &copy
	}
	return err
}

func readCompletionCap(req *model.ChatCompletionRequest, raw json.RawMessage) error {
	if len(raw) > 0 && string(raw) != "null" {
		var cap int
		if err := json.Unmarshal(raw, &cap); err != nil || cap <= 0 {
			return fmt.Errorf("invalid output token cap")
		}
		req.MaxTokens = &cap
	}
	if req.MaxTokens == nil || *req.MaxTokens <= 0 {
		cap := defaultPrepaidCompletionTokens
		req.MaxTokens = &cap
	}
	return nil
}
