package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
	request := &model.ChatCompletionRequest{Model: r.model, Native: &model.NativeRequest{Protocol: model.NativeGeminiInteractions}, Messages: []model.ChatMessage{{Role: "user", Content: input.Input}}}
	topUpQuotaForRequest(c, request)
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
	RecordUsage(c, AuditRecordParams{ChatID: id, Model: r.model, Channel: r.channel.Name, SessionID: resolveGenericSessionID(c, "google"),
		BillingID:    nativeBillingID(r.namespace, id, callerOwner(c)),
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, CachedTokens: usage.GetCachedTokens(), TotalTokens: usage.TotalTokens,
		Duration: duration, TTFT: duration, StatusCode: http.StatusOK, UsageSource: source})
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
		if interaction.Status == "completed" || interaction.Status == "requires_action" {
			if claimGoogleUsage(c, r, interaction.ID) {
				recordGoogleUsage(c, r, interaction.ID, provider.GeminiInteractionUsage(interaction.Usage), time.Since(start))
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
