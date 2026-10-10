package api

import (
	"net/http"
	"os"
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

// HandleMetrics handles GET /metrics.
func (h *Handler) HandleMetrics(c *gin.Context) {
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(telemetry.GlobalMetrics.ToPrometheusFormat()))
}

// HandlePublicStatus handles GET /api/v1/public/status for public status page.
func (h *Handler) HandlePublicStatus(c *gin.Context) {
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

		modelStatuses = append(modelStatuses, publicModelStatus{
			Model:     m,
			Modality:  modality,
			Status:    st,
			LatencyMs: 0,
		})
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
			"uptime_pct":                 99.99,
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
