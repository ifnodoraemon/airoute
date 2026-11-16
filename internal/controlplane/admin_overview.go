package controlplane

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// GetStatsOverview returns dashboard overview metrics. Super-admins see the
// whole cluster; regular users only see aggregates for their own API keys.
// When query param "key_id" is supplied, metrics are scoped to that specific key.
func (h *AdminHandler) GetStatsOverview(c *gin.Context) {
	keyParam, _, paramOK := h.statsKeyFilter(c)
	if !paramOK {
		return
	}
	keyRecord, hasKeyID, ok := h.statsKeyIDFilter(c)
	if !ok {
		return
	}
	claims, ok := GetClaimsFromContext(c)
	if ok && claims.Role != "admin" {
		user, err := h.repo.GetUserByUsername(claims.Username)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "账户不存在或暂时无法验证，请重新登录"})
			return
		}
		if hasKeyID {
			// Ownership is decided on the resolved row, not on a client-supplied
			// key string, so another tenant's id is refused exactly like their key.
			if keyRecord.UserID != user.ID {
				c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权查看该密钥指标"})
				return
			}
			stats, err := h.repo.GetStatsOverviewForKeys([]string{keyRecord.Key})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
			return
		}
		keys, err := h.repo.ListHistoricalAPIKeysByUser(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		ownedMap := make(map[string]bool, len(keys))
		names := make([]string, 0, len(keys))
		for _, k := range keys {
			ownedMap[k.Key] = true
			names = append(names, k.Key)
		}
		if keyParam != "" {
			if !ownedMap[keyParam] {
				c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权查看该密钥指标"})
				return
			}
			names = []string{keyParam}
		}
		stats, err := h.repo.GetStatsOverviewForKeys(names)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
		return
	}

	// Super-admin path
	if hasKeyID {
		stats, err := h.repo.GetStatsOverviewForKeys([]string{keyRecord.Key})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
		return
	}
	if keyParam != "" {
		stats, err := h.repo.GetStatsOverviewForKeys([]string{keyParam})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
		return
	}

	stats, err := h.repo.GetStatsOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
}

// GetStatsTimeSeries returns bucketed time series metrics and true latency percentiles across the cluster.
// Supports drill-down to a single key via the "key_id" query param.
func (h *AdminHandler) GetStatsTimeSeries(c *gin.Context) {
	timeframe := c.DefaultQuery("timeframe", "24h")
	if timeframe != "1h" && timeframe != "24h" && timeframe != "7d" {
		timeframe = "24h"
	}
	keyParam, _, paramOK := h.statsKeyFilter(c)
	if !paramOK {
		return
	}
	keyRecord, hasKeyID, ok := h.statsKeyIDFilter(c)
	if !ok {
		return
	}

	var keyNames []string
	claims, ok := GetClaimsFromContext(c)
	if ok && claims.Role != "admin" {
		user, err := h.repo.GetUserByUsername(claims.Username)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "账户不存在或暂时无法验证，请重新登录"})
			return
		}
		if hasKeyID {
			if keyRecord.UserID != user.ID {
				c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权查看该密钥时序数据"})
				return
			}
			keyNames = []string{keyRecord.Key}
		} else {
			keys, err := h.repo.ListHistoricalAPIKeysByUser(user.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if len(keys) == 0 {
				// Fail-closed: user owns no keys, return empty time series without exposing cluster logs
				c.JSON(http.StatusOK, gin.H{"code": 0, "data": storage.NewEmptyTimeSeriesData(timeframe)})
				return
			}
			ownedMap := make(map[string]bool, len(keys))
			for _, k := range keys {
				ownedMap[k.Key] = true
				keyNames = append(keyNames, k.Key)
			}
			if keyParam != "" {
				if !ownedMap[keyParam] {
					c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "无权查看该密钥时序数据"})
					return
				}
				keyNames = []string{keyParam}
			}
		}
	} else if hasKeyID {
		// Super-admin drilling down into a specific key
		keyNames = []string{keyRecord.Key}
	} else if keyParam != "" {
		keyNames = []string{keyParam}
	}

	data, err := h.repo.GetStatsTimeSeries(timeframe, keyNames)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

// MiddlewareComponentStatus represents the live status of an internal middleware/infrastructure component.
type MiddlewareComponentStatus struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Status      string  `json:"status"` // "operational", "degraded", "disabled"
	Mode        string  `json:"mode"`
	LatencyMs   float64 `json:"latency_ms,omitempty"`
	Description string  `json:"description"`
}

// GetSystemMiddlewares returns real-time health and topology for all underlying infrastructure and middlewares.
//
// It is reserved for ROADMAP B-07's console self-check page (which would also use
// /health/ready). The console has no view for it yet, so the endpoint currently
// has no front-end consumer: it is served for the planned page and for scripted
// diagnostics, not because a shipped screen needs it. If B-07 is dropped, this
// handler and its route should go with it.
// ListModels returns all configured models.
func (h *AdminHandler) ListModels(c *gin.Context) {
	models := h.dispatcher.GetAllSupportedModels()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": models, "thinking": h.dispatcher.GetThinkingCapabilities(models)})
}

// GetStorageStatus returns current artifact storage configuration and health.
func (h *AdminHandler) GetStorageStatus(c *gin.Context) {
	stor := h.getStorage()
	cfg := config.GetGlobalConfig().GetStorageConfig()

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"driver":            stor.Driver(),
			"local_path":        cfg.LocalPath,
			"s3_endpoint":       cfg.S3.Endpoint,
			"s3_bucket":         cfg.S3.Bucket,
			"s3_region":         cfg.S3.Region,
			"s3_use_ssl":        cfg.S3.UseSSL,
			"s3_path_style":     cfg.S3.PathStyle,
			"public_url_prefix": cfg.S3.PublicURLPrefix,
		},
	})
}
