package api

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func videoOwner(c *gin.Context) string {
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

func (h *MultimodalHandler) requireVideoOwner(c *gin.Context) (string, bool) {
	owner := videoOwner(c)
	if owner == "" {
		RespondOpenAIError(c, http.StatusUnauthorized, "Video tasks require an API key", "authentication_error", "api_key_required")
		return "", false
	}
	if h.repo == nil {
		RespondOpenAIError(c, http.StatusServiceUnavailable, "Video task storage is unavailable", "server_error", "task_storage_unavailable")
		return "", false
	}
	return owner, true
}

// HandleVideoGenerations assigns a gateway task ID and persists the chosen provider.
func (h *MultimodalHandler) HandleVideoGenerations(c *gin.Context) {
	owner, ok := h.requireVideoOwner(c)
	if !ok {
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondOpenAIError(c, 400, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}
	var req model.VideoGenerationRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		RespondOpenAIError(c, 400, "A video model is required", "invalid_request_error", "invalid_model")
		return
	}
	if !validateModelCurrency(c, req.Model) {
		return
	}
	if !middleware.ValidateModelAllowed(c, req.Model) {
		RespondOpenAIError(c, 403, "Model is not allowed for your API key", "forbidden", "model_not_allowed")
		return
	}
	h.dispatchAndTransform(c, &router.UpstreamRequest{
		Path: "/v1/videos/generations", Method: http.MethodPost, Body: body,
		ContentType: "application/json", Model: req.Model, Protocol: "videos",
	}, resolveGenericSessionID(c, "vid"), func(resp *router.UpstreamResponse) error {
		payload, err := readVideoResponse(resp)
		if err != nil {
			return err
		}
		upstreamID := videoResponseID(payload)
		if upstreamID != "" {
			task := &storage.VideoTask{ID: "vid_" + storage.NewBillingID(), UpstreamID: upstreamID, Owner: owner, Model: req.Model,
				ChannelID: resp.ChannelID, ChannelName: resp.ProviderName, BaseURL: resp.BaseURL}
			if err := h.repo.SaveVideoTask(c.Request.Context(), task); err != nil {
				return err
			}
			replaceVideoResponseID(payload, task.ID)
		}
		return writeVideoResponse(resp, payload)
	})
}

// HandleVideoTask trusts only the stored model and provider and never load balances a task query.
func (h *MultimodalHandler) HandleVideoTask(c *gin.Context) {
	owner, ok := h.requireVideoOwner(c)
	if !ok {
		return
	}
	task, err := h.repo.GetVideoTask(c.Request.Context(), c.Param("id"), owner)
	if errors.Is(err, sql.ErrNoRows) {
		RespondOpenAIError(c, 404, "Video task not found", "invalid_request_error", "task_not_found")
		return
	}
	if err != nil {
		RespondOpenAIError(c, 503, "Video task storage unavailable", "server_error", "task_storage_unavailable")
		return
	}
	if requested := c.Query("model"); requested != "" && requested != task.Model {
		RespondOpenAIError(c, 400, "Model does not match the video task", "invalid_request_error", "task_model_mismatch")
		return
	}
	if !middleware.ValidateModelAllowed(c, task.Model) {
		RespondOpenAIError(c, 403, "Model is not allowed for your API key", "forbidden", "model_not_allowed")
		return
	}
	h.dispatchAndTransform(c, &router.UpstreamRequest{
		Path: "/v1/videos/tasks/" + url.PathEscape(task.UpstreamID), Method: http.MethodGet, ContentType: "application/json",
		Model: task.Model, Protocol: "videos", PinnedChannel: task.ChannelName, PinnedChannelID: task.ChannelID, PinnedBaseURL: task.BaseURL,
	}, resolveGenericSessionID(c, "vid"), func(resp *router.UpstreamResponse) error {
		payload, err := readVideoResponse(resp)
		if err != nil {
			return err
		}
		replaceVideoResponseID(payload, task.ID)
		return writeVideoResponse(resp, payload)
	})
}

func readVideoResponse(resp *router.UpstreamResponse) (map[string]any, error) {
	const limit = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Stream, limit+1))
	resp.Stream.Close()
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("video response exceeds limit")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("empty video response")
	}
	return payload, nil
}
func videoResponseID(payload map[string]any) string {
	for _, key := range []string{"id", "task_id"} {
		if id, ok := payload[key].(string); ok && id != "" {
			return id
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return videoResponseID(data)
	}
	return ""
}
func replaceVideoResponseID(payload map[string]any, id string) {
	payload["id"] = id
	if _, ok := payload["task_id"]; ok {
		payload["task_id"] = id
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if _, hasID := data["id"]; hasID {
			data["id"] = id
		}
		if _, hasID := data["task_id"]; hasID {
			data["task_id"] = id
		}
	}
}
func writeVideoResponse(resp *router.UpstreamResponse, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp.Stream = io.NopCloser(bytes.NewReader(body))
	resp.Headers = resp.Headers.Clone()
	resp.Headers.Del("Content-Encoding")
	resp.Headers.Del("Transfer-Encoding")
	resp.Headers.Set("Content-Type", "application/json")
	resp.Headers.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}
