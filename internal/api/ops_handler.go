package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/buildinfo"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Readiness checks are the difference between "the process is alive" and "this
// replica can serve traffic". /health is a cheap liveness answer used by the
// edge and by humans; /health/ready additionally pings the dependencies that the
// request path needs, so an orchestrator (or scripts/docker-rebuild.sh, which
// gates rolling restarts on the container health status) never routes to a
// replica whose database or Redis is unreachable.
//
// The registry lives in this package instead of on Handler so cmd/airoute can
// register probes without changing NewHandler's signature.
var (
	readinessMu     sync.RWMutex
	readinessChecks []readinessCheck
)

type readinessCheck struct {
	name string
	fn   func(context.Context) error
}

// readinessCheckTimeout bounds a single /health/ready evaluation: a hung
// dependency must not hang the probe (kubelet would simply report 504).
const readinessCheckTimeout = 2 * time.Second

// RegisterReadinessCheck adds a dependency probe used by GET /health/ready.
func RegisterReadinessCheck(name string, fn func(context.Context) error) {
	if name == "" || fn == nil {
		return
	}
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readinessChecks = append(readinessChecks, readinessCheck{name: name, fn: fn})
}

// ResetReadinessChecks removes every registered probe. Tests use it to isolate
// cases; production code registers once at startup.
func ResetReadinessChecks() {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readinessChecks = nil
}

// HandleReadiness handles GET /health/ready. It answers 200 only when the data
// plane has loaded a configuration snapshot and every registered dependency
// probe succeeds; otherwise 503 with a per-dependency breakdown.
func (h *Handler) HandleReadiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessCheckTimeout)
	defer cancel()

	dependencies := make(map[string]string, 4)
	ready := true

	if middleware.DataPlaneReady() {
		dependencies["data_plane"] = "ok"
	} else {
		dependencies["data_plane"] = "not_loaded"
		ready = false
	}

	readinessMu.RLock()
	checks := append([]readinessCheck(nil), readinessChecks...)
	readinessMu.RUnlock()

	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			// Do not echo the raw error: ping errors can contain connection
			// strings with credentials, and this endpoint is unauthenticated.
			dependencies[check.name] = "unavailable"
			ready = false
			telemetry.WarnLogger(telemetry.ScopedLogger("system"), "readiness:"+check.name, 30*time.Second,
				"readiness probe failed", "dependency", check.name, "error", err.Error())
		} else {
			dependencies[check.name] = "ok"
		}
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}
	c.JSON(status, gin.H{
		"status":       state,
		"service":      "airoute",
		"version":      buildinfo.Version,
		"timestamp":    time.Now().Unix(),
		"dependencies": dependencies,
	})
}

// HandleHealth handles GET /health.
func (h *Handler) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "airoute",
		"version":   buildinfo.Version,
		"timestamp": time.Now().Unix(),
	})
}

// HandleV1Info handles GET /v1 and GET /v1/ - returns service capabilities directory.
func (h *Handler) HandleV1Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "airoute",
		"status":  "operational",
		"version": buildinfo.Version,
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
		"version": buildinfo.Version,
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
		// Read-only listing: GetChannelsForModel would run the SWRR/affinity
		// selector and mutate load-balancing state from an anonymous endpoint.
		channels := h.dispatcher.ListChannelsForModel(m, "")
		healthyCount := 0
		for _, ch := range channels {
			if h.dispatcher.GetBreakerStatus(ch.Name) != "OPEN" {
				healthyCount++
			}
		}

		st := "operational"
		if len(channels) == 0 || healthyCount == 0 {
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
			"stripe_enabled":             strings.TrimSpace(os.Getenv("STRIPE_API_KEY")) != "" && strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")) != "",
			"sandbox_recharge_enabled":   os.Getenv("ENABLE_SANDBOX_RECHARGE") == "true",
		},
	})
}
