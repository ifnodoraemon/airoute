package controlplane

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// statsKeyIDFilter resolves the dashboard's key drill-down filter.
//
// The console drills into a key by its numeric id (`key_id`), because the key
// list only exposes a masked display value and the fingerprint — never the stored
// digest. Resolving the id here, against the authoritative row, is what lets the
// repository be queried with the digest that usage_logs actually stores; the old
// `key=` filter was fed the masked value, which hashed to a digest that matched
// no row, so a drill-down showed zero traffic for a busy key.
//
// It returns (record, present, ok). present reports whether the parameter was
// supplied; ok is false when a response has already been written.
func (h *AdminHandler) statsKeyIDFilter(c *gin.Context) (*storage.APIKeyRecord, bool, bool) {
	raw := strings.TrimSpace(c.Query("key_id"))
	if raw == "" {
		return nil, false, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的密钥 ID"})
		return nil, true, false
	}
	record, err := h.repo.GetAPIKey(id)
	if err != nil || record == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "未找到指定 API 密钥"})
		return nil, true, false
	}
	return record, true, true
}

// GetStatsOverview returns dashboard overview metrics. Super-admins see the
// whole cluster; regular users only see aggregates for their own API keys.
// When query param "key_id" is supplied, metrics are scoped to that specific key.
func (h *AdminHandler) GetStatsOverview(c *gin.Context) {
	keyParam := strings.TrimSpace(c.Query("key"))
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
	keyParam := strings.TrimSpace(c.Query("key"))
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
func (h *AdminHandler) GetSystemMiddlewares(c *gin.Context) {
	var list []MiddlewareComponentStatus

	// 1. Database
	dbStart := time.Now()
	dbStatus := "operational"
	dialect := "SQLite"
	if h.repo != nil {
		if strings.EqualFold(h.repo.Dialect(), "postgres") {
			dialect = "PostgreSQL"
		}
		if db := h.repo.GetDB(); db != nil {
			if err := db.Ping(); err != nil {
				dbStatus = "degraded"
			}
		}
	}
	dbLatency := float64(time.Since(dbStart).Microseconds()) / 1000.0
	list = append(list, MiddlewareComponentStatus{
		ID:          "database",
		Name:        fmt.Sprintf("持久化存储引擎 (%s)", dialect),
		Category:    "数据存储",
		Status:      dbStatus,
		Mode:        fmt.Sprintf("%s 关系引擎 · 自动二级索引加速", dialect),
		LatencyMs:   dbLatency,
		Description: "承载用户档案、渠道配置、多阶梯计费策略与全量调用审计流水的可靠存储与高并发读取。",
	})

	// 2. Distributed Cache & Cluster Synchronizer (Redis)
	redisClient := distributed.GetClient()
	if redisClient != nil && redisClient.IsActive() {
		list = append(list, MiddlewareComponentStatus{
			ID:          "redis",
			Name:        "分布式缓存与集群同步 (Redis)",
			Category:    "缓存与集群协调",
			Status:      "operational",
			Mode:        "Redis 集中式集群 · Pub/Sub 热重载总线",
			Description: "支持多节点热重载信号秒级下发、会话粘性保持与分布式并发限流协同。",
		})
	} else {
		list = append(list, MiddlewareComponentStatus{
			ID:          "redis",
			Name:        "进程内高速缓存与状态机",
			Category:    "缓存与状态机",
			Status:      "operational",
			Mode:        "零依赖本地内存模式 (In-Memory)",
			Description: "单节点极速低延迟内存缓存；如需多机无状态部署，设置 REDIS_URL 环境变量即可无缝扩容。",
		})
	}

	// 3. Message Stream & Async Queue
	if redisClient != nil && redisClient.IsActive() {
		list = append(list, MiddlewareComponentStatus{
			ID:          "message_stream",
			Name:        "分布式事件流队列 (Redis Stream)",
			Category:    "消息流管道",
			Status:      "operational",
			Mode:        "Redis Stream (airoute:stream:logs) 消费组",
			Description: "纳秒级非阻塞事件追加，消费者组分布式并发消费，确保高峰期高并发日志不丢不漏。",
		})
	} else {
		list = append(list, MiddlewareComponentStatus{
			ID:          "message_stream",
			Name:        "异步流水队列与消费缓冲",
			Category:    "消息管道与缓冲",
			Status:      "operational",
			Mode:        "Go 环形异步缓冲管道 (5,000 缓冲深度)",
			Description: "全内存异步批处理聚合并发入库，保障 API 请求数据转发主链路零阻塞、零额外时延。",
		})
	}

	// 4. HA Circuit Breaker & Dispatcher
	channels := h.dispatcher.GetAllSupportedModels()
	list = append(list, MiddlewareComponentStatus{
		ID:          "circuit_breaker",
		Name:        "高可用容灾与熔断调度器",
		Category:    "可靠性与路由",
		Status:      "operational",
		Mode:        "Safe Fallback Window + 自适应熔断 + 循环检测",
		Description: fmt.Sprintf("统一调度 %d 个模型路由与多渠道优先级负载均衡，具备首字前毫秒级兜底与防死循环保护。", len(channels)),
	})

	// 5. Billing & Off-Peak Engine
	list = append(list, MiddlewareComponentStatus{
		ID:          "billing_engine",
		Name:        "多分组计费与优惠时段引擎",
		Category:    "计量与计费",
		Status:      "operational",
		Mode:        "多分组阶梯定价 + DeepSeek 闲时自动半价调度",
		Description: "支持多分组定价阶梯、Prompt Cache 代币节约精准统计与多模态全模态费率核算。",
	})

	// 6. Notification & SMTP Service
	smtpMode := "未配置 SMTP，邮件验证码不可用"
	smtpStatus := "disabled"
	if strings.TrimSpace(os.Getenv("SMTP_HOST")) != "" {
		smtpMode = fmt.Sprintf("SMTP 邮件中继服务 (%s)", os.Getenv("SMTP_HOST"))
		smtpStatus = "configured"
	}
	list = append(list, MiddlewareComponentStatus{
		ID:          "notification",
		Name:        "用户通知与安全验证系统",
		Category:    "通知与认证",
		Status:      smtpStatus,
		Mode:        smtpMode,
		Description: "用于发送邮箱验证码；配置状态不代表邮件已送达，请通过发送验证码验证实际投递。",
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": list,
	})
}

// ListModels returns all configured models.
func (h *AdminHandler) ListModels(c *gin.Context) {
	models := h.dispatcher.GetAllSupportedModels()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": models})
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
