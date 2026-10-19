package api

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// HandleHealth handles GET /health.
func (h *Handler) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "airoute",
		"version":   "0.1.0",
		"timestamp": time.Now().Unix(),
	})
}

// HandleV1Info handles GET /v1 and GET /v1/ - returns service capabilities directory.
func (h *Handler) HandleV1Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "airoute",
		"status":  "operational",
		"version": "0.1.0",
		"message": "Airoute High-Performance AI Router & Multimodal Gateway API v1",
		"protocols": []string{
			"OpenAI Chat Completions (/v1/chat/completions)",
			"OpenAI Responses API (/v1/responses)",
			"OpenAI Text Completions (/v1/completions)",
			"OpenAI Models Registry (/v1/models)",
			"OpenAI Embeddings (/v1/embeddings)",
			"OpenAI Moderations (/v1/moderations)",
			"Cohere/BGE Semantic Rerank (/v1/rerank)",
			"Anthropic Claude Messages (/v1/messages)",
			"Google Gemini Models (/v1beta/models)",
			"Multimodal Ingress (/v1/images/generations, /v1/audio/speech, /v1/audio/transcriptions, /v1/videos/generations)",
			"Model Context Protocol (MCP 2024-11-05) (/v1/mcp, /v1/mcp/sse, /v1/mcp/messages)",
		},
		"endpoints": gin.H{
			"chat_completions":     "/v1/chat/completions",
			"completions":          "/v1/completions",
			"responses":            "/v1/responses",
			"models":               "/v1/models",
			"embeddings":           "/v1/embeddings",
			"rerank":               "/v1/rerank",
			"messages":             "/v1/messages",
			"images":               "/v1/images/generations",
			"audio_speech":         "/v1/audio/speech",
			"audio_transcriptions": "/v1/audio/transcriptions",
			"audio_translations":   "/v1/audio/translations",
			"videos":               "/v1/videos/generations",
			"mcp":                  "/v1/mcp",
		},
		"documentation": "/app/",
	})
}

// HandleV1BetaInfo handles GET /v1beta and GET /v1beta/ - returns Gemini ingress info.
func (h *Handler) HandleV1BetaInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "airoute",
		"status":  "operational",
		"version": "0.1.0",
		"message": "Airoute Google Gemini Protocol Ingress v1beta",
		"endpoints": gin.H{
			"models": "/v1beta/models",
		},
		"documentation": "/app/",
	})
}

// HandleMetrics handles GET /metrics.
func (h *Handler) HandleMetrics(c *gin.Context) {
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(telemetry.GlobalMetrics.ToPrometheusFormat()))
}

// HandlePublicStatus handles GET /api/v1/public/status for public status page.
// Set GATEWAY_PUBLIC_STATUS=false to hide the model catalog from anonymous
// visitors (the endpoint then reports only overall availability).
func (h *Handler) HandlePublicStatus(c *gin.Context) {
	exposeModels := !strings.EqualFold(os.Getenv("GATEWAY_PUBLIC_STATUS"), "false")
	models := h.dispatcher.GetAllSupportedModels()
	type publicModelStatus struct {
		Model     string `json:"model"`
		Modality  string `json:"modality"`
		Status    string `json:"status"` // "operational" | "degraded"
		LatencyMs int64  `json:"latency_ms"`
	}

	var modelStatuses []publicModelStatus
	allHealthy := true

	for _, m := range models {
		channels := h.dispatcher.GetChannelsForModel(m)
		healthyCount := 0
		for _, ch := range channels {
			if h.dispatcher.GetBreakerStatus(ch.Name) != "OPEN" {
				healthyCount++
			}
		}

		st := "operational"
		if healthyCount == 0 && len(channels) > 0 {
			st = "degraded"
			allHealthy = false
		}

		modality := router.InferModality(m)

		if exposeModels {
			modelStatuses = append(modelStatuses, publicModelStatus{
				Model:     m,
				Modality:  modality,
				Status:    st,
				LatencyMs: 0,
			})
		}
	}

	overallStatus := "operational"
	if !allHealthy && len(models) > 0 {
		overallStatus = "degraded"
	}

	cfg := config.GetGlobalConfig()
	requireVerify := cfg.IsEmailVerificationRequired()
	allowReg := cfg.IsRegistrationAllowed()
	baseURL := controlplane.ResolvePublicBaseURL(c)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"status":                     overallStatus,
			"public_url":                 baseURL,
			"models":                     modelStatuses,
			"models_count":               len(models),
			"allow_registration":         allowReg,
			"require_email_verification": requireVerify,
			"oauth_github_enabled":       os.Getenv("GITHUB_CLIENT_ID") != "",
			"oauth_google_enabled":       os.Getenv("GOOGLE_CLIENT_ID") != "",
			"stripe_enabled":             os.Getenv("STRIPE_API_KEY") != "",
			"sandbox_recharge_enabled":   os.Getenv("ENABLE_SANDBOX_RECHARGE") == "true",
		},
	})
}
