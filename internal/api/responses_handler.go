package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	// responseContextStoreTimeout bounds the previous-response context lookup and
	// save on the request path. The database is local, so this only fires when
	// something is wrong; a stalled store must not pin the request.
	responseContextStoreTimeout = 2 * time.Second

	// maxResponseContextBytes caps what a single conversation may store. The
	// conversation is replayed verbatim for a week, and a message can carry
	// inline base64 images; refusing a pathological payload is cheaper than
	// letting it bloat the database. Such a response simply is not resumable.
	maxResponseContextBytes = 4 << 20
)

// HandleResponses handles OpenAI Responses API POST /v1/responses.
func (h *Handler) HandleResponses(c *gin.Context) {
	var req model.ResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if respondRequestTooLarge(c, err) {
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON request body: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_payload",
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing 'model' in request",
				"type":    "invalid_request_error",
				"code":    "missing_model",
			},
		})
		return
	}

	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' not allowed for this key", req.Model),
				"type":    "forbidden",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "invalid_request_error",
				"code":    "invalid_input",
			},
		})
		return
	}

	// A follow-up that references a previous turn must actually replay that
	// conversation. Silently ignoring previous_response_id made every such request
	// answer with no history at all, which looked like the model had amnesia.
	if previousID := strings.TrimSpace(req.PreviousResponseID); previousID != "" {
		previousMessages, ok := restoreResponseContext(c, previousID)
		if !ok {
			return
		}
		chatReq.Messages = model.MergeResponseConversation(chatReq.Messages, previousMessages)
	}

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()

	sessionID := sanitizeSessionID(req.PreviousResponseID)
	if sessionID == "" {
		sessionID = resolveSessionID(c, chatReq.Model, chatReq.Messages, req.User)
	} else {
		sessionID = "resp_" + sessionID
		publishSession(c, sessionID)
	}
	// The provider call is detached from the downstream request so a customer
	// disconnect cannot cancel it before its usage report arrives.
	reqCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()
	if sessionID != "" {
		reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionAffinityKey(c, sessionID))
	}

	start := time.Now()

	// Non-streaming execution
	if !req.Stream {
		h.handleResponsesNonStreaming(c, reqCtx, req, chatReq, sessionID, start)
		return
	}

	// Streaming SSE execution
	h.handleResponsesStreaming(c, reqCtx, req, chatReq, sessionID, start)
}

func (h *Handler) handleResponsesNonStreaming(
	c *gin.Context,
	reqCtx context.Context,
	req model.ResponseRequest,
	chatReq *model.ChatCompletionRequest,
	sessionID string,
	start time.Time,
) {
	resp, err := h.dispatcher.Dispatch(reqCtx, chatReq)
	if err != nil {
		respondUpstreamDispatchError(c, sessionID, req.Model, time.Since(start), err)
		return
	}

	dur := time.Since(start)
	pTokens, cTokens, cachedTokens, usageSource := resolveChatUsage(chatReq, resp)

	RecordUsage(c, AuditRecordParams{
		ChatID:           resp.ID,
		Channel:          resp.Channel,
		SessionID:        sessionID,
		Model:            req.Model,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		CachedTokens:     cachedTokens,
		Duration:         dur,
		StatusCode:       http.StatusOK,
		UsageSource:      usageSource,
	})

	result := model.ConvertChatResponseToResponseResult(resp)
	// Persist before the client sees the response id: a follow-up that uses
	// previous_response_id can arrive immediately, and it must already resolve.
	// A persistence failure is logged, never surfaced: the answer itself is valid.
	persistResponseContext(c, result.ID, chatReq.Messages, model.AssistantMessageFromChatResponse(resp))
	c.JSON(http.StatusOK, result)
}

// responseContextOwner identifies the caller that owns a stored Responses
// conversation: the authenticated user when the API key belongs to one,
// otherwise the API-key digest. Two credentials must never share a conversation.
func responseContextOwner(c *gin.Context) string {
	key := getAPIKeyConfig(c)
	if key == nil || key.Key == "" {
		return ""
	}
	if key.UserID > 0 {
		return "user:" + strconv.FormatInt(key.UserID, 10)
	}
	digest := sha256.Sum256([]byte(key.Key))
	return "key:" + hex.EncodeToString(digest[:])
}

// restoreResponseContext loads the conversation behind a previous_response_id
// for the current caller. On any unresolvable id it writes the documented 400
// and reports false; quietly continuing without history is exactly the bug this
// replaces.
func restoreResponseContext(c *gin.Context, previousID string) ([]model.ChatMessage, bool) {
	owner := responseContextOwner(c)
	repo := storage.GetGlobalRepository()
	if repo == nil || owner == "" {
		respondPreviousResponseNotFound(c, previousID)
		return nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), responseContextStoreTimeout)
	defer cancel()
	stored, err := repo.LoadResponseContext(ctx, previousID, owner)
	if err != nil {
		if !errors.Is(err, storage.ErrResponseContextNotFound) {
			telemetry.WarnLogger(telemetry.ScopedLogger("system"), "responses-context-load-failed", time.Minute,
				"failed to load the Responses previous context", "response_id", previousID, "error", err.Error())
		}
		respondPreviousResponseNotFound(c, previousID)
		return nil, false
	}
	var messages []model.ChatMessage
	if err := json.Unmarshal(stored.Messages, &messages); err != nil {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "responses-context-decode-failed", time.Minute,
			"stored Responses previous context is not decodable", "response_id", previousID, "error", err.Error())
		respondPreviousResponseNotFound(c, previousID)
		return nil, false
	}
	return messages, true
}

// persistResponseContext stores the conversation of a completed response under
// its client-facing id so a later request can reference it. It never fails the
// response: storage trouble must not turn a successful provider answer into an
// error.
func persistResponseContext(c *gin.Context, responseID string, conversation []model.ChatMessage, assistant *model.ChatMessage) {
	owner := responseContextOwner(c)
	repo := storage.GetGlobalRepository()
	if repo == nil || owner == "" || responseID == "" {
		return
	}
	payload, err := json.Marshal(model.ResponseConversationMessages(conversation, assistant))
	if err != nil {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "responses-context-encode-failed", time.Minute,
			"failed to encode the Responses conversation for persistence", "response_id", responseID, "error", err.Error())
		return
	}
	if len(payload) > maxResponseContextBytes {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "responses-context-too-large", time.Minute,
			"skipped persisting a Responses conversation that exceeds the context size cap",
			"response_id", responseID, "bytes", len(payload), "cap_bytes", maxResponseContextBytes)
		return
	}
	// Detached from the request context: a downstream disconnect right after the
	// answer must not drop the conversation the next turn depends on.
	ctx, cancel := context.WithTimeout(context.Background(), responseContextStoreTimeout)
	defer cancel()
	if err := repo.SaveResponseContext(ctx, responseID, owner, payload); err != nil {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "responses-context-save-failed", time.Minute,
			"failed to persist the Responses conversation", "response_id", responseID, "error", err.Error())
	}
}

// respondPreviousResponseNotFound rejects an unresolvable previous_response_id
// with the documented invalid_request_error.
func respondPreviousResponseNotFound(c *gin.Context, previousID string) {
	RespondOpenAIError(c, http.StatusBadRequest,
		fmt.Sprintf("Previous response %q was not found. It may have expired (context is kept for %d days) or it belongs to a different API key.",
			sanitizeSessionID(previousID), int(storage.ResponseContextTTL/(24*time.Hour))),
		"invalid_request_error", "previous_response_not_found")
}
