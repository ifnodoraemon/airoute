package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func (h *Handler) handleGeminiInteraction(c *gin.Context, r *googleResource) {
	start := time.Now()
	ctx, cancel := upstreamRequestContext(c)
	defer cancel()
	r.context = ctx
	var input struct {
		Model    string `json:"model"`
		Agent    string `json:"agent"`
		Previous string `json:"previous_interaction_id"`
		Stream   bool   `json:"stream"`
		Input    any    `json:"input"`
	}
	if len(r.body) > 0 && json.Unmarshal(r.body, &input) != nil {
		RespondGeminiError(c, 400, "Invalid interaction JSON", "INVALID_ARGUMENT")
		return
	}
	if input.Previous != "" {
		previous, err := nativestate.Load(c.Request.Context(), "gemini-interaction", input.Previous, callerOwner(c))
		if err != nil {
			RespondGeminiError(c, 404, "Previous interaction not found", "NOT_FOUND")
			return
		}
		r.state = previous
		if r.model == "" {
			r.model = previous.Model
		}
	}
	if r.model == "" {
		r.model = input.Agent
	}
	if r.model == "" || !h.googleModelAllowed(c, r.model) {
		RespondGeminiError(c, 403, "Interaction model is not allowed", "PERMISSION_DENIED")
		return
	}
	if !validateModelCurrency(c, r.model) {
		return
	}
	request := &model.ChatCompletionRequest{Model: r.model, Native: &model.NativeRequest{Protocol: model.NativeGeminiInteractions, Body: r.body}, Messages: []model.ChatMessage{{Role: "user", Content: input.Input}}}
	if input.Agent != "" && input.Model == "" && middleware.RequestQuotaFrom(c) != nil {
		if !requireFlatFeeAgentQuota(c, r.model) {
			return
		}
	} else {
		if !topUpQuotaForRequest(c, request) {
			return
		}
	}
	r.body = request.Native.Body
	response, err := h.googleResourceCall(c, r)
	if err != nil {
		status, _, _, _ := resolveUpstreamFailureStatus(err)
		recordFailedRequestWithError(c, "", r.model, time.Since(start), status, err)
		RespondGeminiError(c, status, sanitizeErrorMessage(err), geminiErrorStatus(status))
		return
	}
	defer response.Stream.Close()
	if strings.Contains(response.Headers.Get("Content-Type"), "text/event-stream") || input.Stream {
		h.forwardInteractionStream(c, r, request, response.Stream, start)
		return
	}
	h.forwardInteractionResponse(c, r, response, start)
}

func (h *Handler) forwardInteractionStream(c *gin.Context, r *googleResource, request *model.ChatCompletionRequest, body io.ReadCloser, start time.Time) {
	ctx, cancel := upstreamRequestContext(c)
	defer cancel()
	in := provider.ReadNativeProtocol(ctx, body, model.NativeGeminiInteractions, r.model, r.channel)
	out := make(chan *model.StreamEvent, 64)
	id := ""
	terminal, claimed := false, false
	request.Native.ShouldRecord = func() bool {
		if !terminal || id == "" {
			return false
		}
		request.Native.BillingID = nativeBillingID(r.namespace, id, callerOwner(c))
		claimed = claimGoogleUsage(c, r, id)
		return claimed
	}
	request.Native.AfterRecord = func() {
		if claimed {
			completeGoogleUsage(c, r, id)
		}
	}
	go func() {
		defer close(out)
		for event := range in {
			event.Channel = r.channel.Name
			if event.Chunk != nil && event.Chunk.ID != "" {
				current := event.Chunk.ID
				wire := append([]byte(nil), event.NativeEvent...)
				done := event.IsDone
				event.BeforeForward = func() error {
					id = current
					terminal = terminal || done
					data := interactionFromSSE(wire)
					if len(data) == 0 {
						data, _ = json.Marshal(map[string]string{"id": current, "model": r.model})
					}
					return saveGoogleResource(c, r, current, data)
				}
			}
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	var first *model.StreamEvent
	select {
	case first = <-out:
	case <-ctx.Done():
		return
	}
	if first == nil {
		RespondGeminiError(c, 502, "Empty interaction stream", "UNAVAILABLE")
		return
	}
	h.forwardNativeStream(c, request, out, first, resolveGenericSessionID(c, "interaction"), start)
}

func claimGoogleUsage(c *gin.Context, r *googleResource, id string) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
	defer cancel()
	claimed, err := nativestate.Claim(ctx, r.namespace, id, callerOwner(c))
	return err == nil && claimed
}

func recordGoogleUsage(c *gin.Context, r *googleResource, id string, usage *model.Usage, duration time.Duration) {
	recordGoogleUsageSource(c, r, id, usage, duration, usageSourceUpstream)
}

func recordGoogleUsageSource(c *gin.Context, r *googleResource, id string, usage *model.Usage, duration time.Duration, source string) {
	if usage == nil {
		return
	}
	params := googleUsageParams(c, r)
	params.ChatID, params.Model, params.Channel = id, r.model, r.channel.Name
	params.SessionID = resolveGenericSessionID(c, "google")
	params.BillingID = nativeBillingID(r.namespace, id, callerOwner(c))
	params.PromptTokens, params.CompletionTokens, params.CachedTokens, params.TotalTokens = usage.PromptTokens, usage.CompletionTokens, usage.GetCachedTokens(), usage.TotalTokens
	params.Duration, params.TTFT, params.StatusCode, params.UsageSource = duration, duration, http.StatusOK, source
	RecordUsage(c, params)
}

func (h *Handler) forwardInteractionResponse(c *gin.Context, r *googleResource, response *router.UpstreamResponse, start time.Time) {
	data, err := readGoogleJSON(response.Stream)
	if err != nil {
		RespondGeminiError(c, 502, "Failed to read interaction", "UNAVAILABLE")
		return
	}
	var interaction struct {
		ID     string          `json:"id"`
		Status string          `json:"status"`
		Usage  json.RawMessage `json:"usage"`
	}
	_ = json.Unmarshal(data, &interaction)
	if interaction.ID != "" {
		if err := saveGoogleResource(c, r, interaction.ID, data); err != nil {
			RespondGeminiError(c, 503, "Cannot retain interaction ownership", "UNAVAILABLE")
			return
		}
		failed := interaction.Status == "failed" || interaction.Status == "cancelled" || interaction.Status == "canceled" || interaction.Status == "expired"
		if interaction.Status == "completed" || interaction.Status == "requires_action" || failed {
			if claimGoogleUsage(c, r, interaction.ID) {
				usage := provider.GeminiInteractionUsage(interaction.Usage)
				if failed && (usage == nil || usage.TotalTokens == 0) {
					params := googleUsageParams(c, r)
					params.BillingID, params.ChatID, params.Model, params.Channel = nativeBillingID(r.namespace, interaction.ID, callerOwner(c)), interaction.ID, r.model, r.channel.Name
					params.StatusCode, params.SkipBilling = 200, true
					RecordUsage(c, params)
				} else {
					recordGoogleUsage(c, r, interaction.ID, usage, time.Since(start))
				}
				completeGoogleUsage(c, r, interaction.ID)
			}
		}
	}
	if c.Request.Method == http.MethodDelete {
		_ = nativestate.Delete(c.Request.Context(), r.namespace, r.id, callerOwner(c))
	}
	copyGoogleResponseHeaders(c, response.Headers)
	c.Data(response.StatusCode, "application/json", provider.RewriteNativeModel(data, r.model))
}
