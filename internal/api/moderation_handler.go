package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// moderationStubEnabled reports whether the synthetic "everything is safe"
// response is allowed. It is opt-in on purpose: a fabricated verdict silently
// defeats any downstream content filtering, so it must never be the default.
func moderationStubEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_MODERATION_STUB"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ModerationRequest represents the OpenAI moderation request body.
type ModerationRequest struct {
	Input any    `json:"input"`
	Model string `json:"model,omitempty"`
}

// ModerationResult represents an individual moderation evaluation.
type ModerationResult struct {
	Flagged        bool               `json:"flagged"`
	Categories     map[string]bool    `json:"categories"`
	CategoryScores map[string]float64 `json:"category_scores"`
}

// ModerationResponse represents the standard OpenAI moderation response.
type ModerationResponse struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Results []ModerationResult `json:"results"`
}

// HandleModerations handles POST /v1/moderations for OpenAI moderation API clients.
func (h *Handler) HandleModerations(c *gin.Context) {
	var req ModerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid JSON body: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_payload",
			},
		})
		return
	}

	modModel := req.Model
	if modModel == "" {
		modModel = "text-moderation-latest"
	}

	var inputs []string
	switch v := req.Input.(type) {
	case string:
		inputs = append(inputs, v)
	case []any:
		for _, item := range v {
			inputs = append(inputs, fmt.Sprint(item))
		}
	}
	if len(inputs) == 0 {
		inputs = append(inputs, "")
	}

	channels := h.dispatcher.GetChannelsForModelAndProtocol(modModel, "moderation")
	if len(channels) > 0 {
		payloadBytes, _ := json.Marshal(req)
		resp, err := h.dispatcher.DispatchHTTP(c.Request.Context(), &router.UpstreamRequest{
			Path:        "/v1/moderations",
			Method:      http.MethodPost,
			ContentType: "application/json",
			Body:        payloadBytes,
			Model:       modModel,
			Protocol:    "moderation",
		})
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Stream.Close()
			body, _ := io.ReadAll(resp.Stream)
			c.Data(resp.StatusCode, "application/json", body)
			return
		}
		if resp != nil && resp.Stream != nil {
			resp.Stream.Close()
		}
	}

	if !moderationStubEnabled() {
		// Returning a synthetic "everything is safe" verdict is a safety hazard:
		// a caller that trusts this endpoint to filter content would silently
		// receive "unflagged" for every input, with plausible-looking scores and
		// an HTTP 200. Refuse explicitly instead, and say how to fix it.
		telemetry.ScopedLogger("system").Warn("moderation request rejected: no moderation channel configured",
			"model", modModel)
		c.Set(middleware.ContextKeyErrorMessage, "moderation_not_configured")
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"message": "No moderation provider is configured on this gateway. A synthetic verdict is not returned; add a channel that handles the 'moderation' protocol.",
				"type":    "invalid_request_error",
				"code":    "moderation_not_configured",
			},
		})
		return
	}

	// Development-only escape hatch, enabled explicitly with GATEWAY_MODERATION_STUB.
	telemetry.WarnLogger(telemetry.ScopedLogger("system"), "moderation-stub", 5*time.Minute,
		"returning synthetic moderation verdict; GATEWAY_MODERATION_STUB is enabled",
		"model", modModel)

	var results []ModerationResult
	for range inputs {
		results = append(results, ModerationResult{
			Flagged: false,
			Categories: map[string]bool{
				"sexual":                 false,
				"hate":                   false,
				"harassment":             false,
				"self-harm":              false,
				"sexual/minors":          false,
				"hate/threatening":       false,
				"violence/graphic":       false,
				"self-harm/intent":       false,
				"self-harm/instructions": false,
				"harassment/threatening": false,
				"violence":               false,
			},
			CategoryScores: map[string]float64{
				"sexual":                 0.00001,
				"hate":                   0.00001,
				"harassment":             0.00001,
				"self-harm":              0.00001,
				"sexual/minors":          0.00001,
				"hate/threatening":       0.00001,
				"violence/graphic":       0.00001,
				"self-harm/intent":       0.00001,
				"self-harm/instructions": 0.00001,
				"harassment/threatening": 0.00001,
				"violence":               0.00001,
			},
		})
	}

	c.JSON(http.StatusOK, ModerationResponse{
		ID:      fmt.Sprintf("modr-%d", time.Now().UnixMicro()),
		Model:   modModel,
		Results: results,
	})
}
