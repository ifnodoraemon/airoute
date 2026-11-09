package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func (h *Handler) handleGeminiBatchAction(c *gin.Context, name, action string) {
	if callerOwner(c) == "" {
		RespondGeminiError(c, 401, "Authenticated resource owner required", "UNAUTHENTICATED")
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || !json.Valid(body) {
		RespondGeminiError(c, 400, "Invalid batch JSON", "INVALID_ARGUMENT")
		return
	}
	if !validateModelCurrency(c, name) {
		return
	}
	if !reserveGoogleBatch(c, name, action, &body) {
		return
	}

	r := &googleResource{namespace: "gemini-batch", collection: "batches", model: name, body: body, action: action}
	h.forwardGoogleResource(c, r)
}

func (h *Handler) recordGoogleBatchResult(c *gin.Context, r *googleResource, body []byte) {
	if r.namespace != "gemini-batch" {
		return
	}
	usage, terminal := googleBatchUsage(body)
	if r.action == "batchEmbedContents" {
		if usage == nil {
			usage = &model.Usage{PromptTokens: estimateTokens(len(r.body))}
			usage.TotalTokens = usage.PromptTokens
		}
		RecordUsage(c, AuditRecordParams{Model: r.model, Channel: r.channel.Name, PromptTokens: usage.PromptTokens, TotalTokens: usage.TotalTokens,
			StatusCode: 200, UsageSource: usageSourceEstimated})
		return
	}
	if !terminal {
		return
	}
	id := statesID(body)
	if id == "" {
		id = r.id
	}
	if id == "" || !claimGoogleUsage(c, r, id) {
		return
	}
	for _, file := range googleBatchOutputFiles(body) {
		fileUsage, err := h.googleBatchFileUsage(c, r, file)
		if err != nil {
			telemetry.ScopedLogger("billing").Warn("native batch usage file is not yet readable", "job", id, "error", err.Error())
			return
		}
		usage.PromptTokens += fileUsage.PromptTokens
		usage.CompletionTokens += fileUsage.CompletionTokens
		usage.TotalTokens += fileUsage.TotalTokens
	}
	source := usageSourceUpstream
	if usage.TotalTokens == 0 {
		var snapshot struct {
			Tokens int `json:"_airoute_estimated_prompt_tokens"`
		}
		if r.state != nil {
			_ = json.Unmarshal(r.state.Billing, &snapshot)
		}
		usage.PromptTokens = max(snapshot.Tokens, estimateTokens(len(r.body)))
		usage.TotalTokens = usage.PromptTokens
		source = usageSourceEstimated
	}
	recordGoogleUsageSource(c, r, id, usage, 0, source)
	completeGoogleUsage(c, r, id)
}

func googleBatchUsage(body []byte) (*model.Usage, bool) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	usage := &model.Usage{}
	terminal := false
	var walk func(any)
	walk = func(value any) {
		switch object := value.(type) {
		case []any:
			for _, v := range object {
				walk(v)
			}
		case map[string]any:
			if done, _ := object["done"].(bool); done {
				terminal = true
			}
			if state, _ := object["state"].(string); state == "BATCH_STATE_SUCCEEDED" || state == "BATCH_STATE_FAILED" || state == "BATCH_STATE_CANCELLED" || state == "BATCH_STATE_EXPIRED" {
				terminal = true
			}
			if raw, ok := object["usageMetadata"]; ok {
				data, _ := json.Marshal(raw)
				var u provider.GeminiUsageMetadata
				if json.Unmarshal(data, &u) == nil {
					usage.PromptTokens += u.PromptTokenCount
					usage.CompletionTokens += u.CandidatesTokenCount + u.ThoughtsTokenCount
					usage.TotalTokens += u.TotalTokenCount
				}
			}
			for key, v := range object {
				if key != "usageMetadata" {
					walk(v)
				}
			}
		}
	}
	walk(value)
	usage.TotalTokens = max(usage.TotalTokens, usage.PromptTokens+usage.CompletionTokens)
	return usage, terminal
}

func nativeBillingID(namespace, id, owner string) string {
	sum := sha256.Sum256([]byte(namespace + "\x00" + id + "\x00" + owner))
	text := hex.EncodeToString(sum[:16])
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]
}

func completeGoogleUsage(c *gin.Context, r *googleResource, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
	defer cancel()
	_ = nativestate.Complete(ctx, r.namespace, id, callerOwner(c))
}

func interactionFromSSE(wire []byte) json.RawMessage {
	var event struct {
		Interaction json.RawMessage `json:"interaction"`
	}
	_ = json.Unmarshal(googleSSEData(wire), &event)
	return event.Interaction
}

func googleSSEData(wire []byte) []byte {
	if json.Valid(wire) {
		return wire
	}
	var lines []string
	for _, line := range strings.Split(string(wire), "\n") {
		if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
