package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	stdSync "sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/billing"
	"github.com/ifnodoraemon/nano-gateway/internal/distributed"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/provider"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

// AdminHandler handles REST endpoints for the Control Plane.
type AdminHandler struct {
	repo       *storage.Repository
	sync       *Synchronizer
	dispatcher *router.Dispatcher
	prober     *DownstreamProber
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(repo *storage.Repository, sync *Synchronizer, dispatcher *router.Dispatcher) *AdminHandler {
	return &AdminHandler{
		repo:       repo,
		sync:       sync,
		dispatcher: dispatcher,
		prober:     NewDownstreamProber(nil),
	}
}

// syncDataPlane synchronizes updated channels, virtual keys, and routing rules into memory and cluster replicas.
func (h *AdminHandler) syncDataPlane(events ...string) {
	if h.sync != nil {
		event := "data_plane_updated"
		if len(events) > 0 && events[0] != "" {
			event = events[0]
		}
		_ = h.sync.ReloadAndBroadcast(cContext(), event)
	}
}

func cContext() context.Context {
	return context.Background()
}

// ProbeChannel handles automated downstream service detection and discovery.
func (h *AdminHandler) ProbeChannel(c *gin.Context) {
	var req ProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	telemetry.Logger.Info("handling downstream channel probe request",
		"base_url", req.BaseURL,
		"has_api_key", req.APIKey != "",
		"type", req.Type,
		"client_ip", c.ClientIP(),
	)

	result, err := h.prober.Probe(c.Request.Context(), &req)
	if err != nil {
		telemetry.Logger.Warn("downstream channel probe returned error",
			"base_url", req.BaseURL,
			"error", err.Error(),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	telemetry.Logger.Info("downstream channel probe completed",
		"base_url", req.BaseURL,
		"detected_type", result.Type,
		"suggested_name", result.SuggestedName,
		"suggested_base_url", result.SuggestedBaseURL,
		"models_count", len(result.Models),
		"latency_ms", result.LatencyMs,
		"message", result.Message,
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// ListChannels returns all configured channels.
func (h *AdminHandler) ListChannels(c *gin.Context) {
	channels, err := h.repo.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.dispatcher != nil {
		for _, ch := range channels {
			ch.BreakerStatus = h.dispatcher.GetBreakerStatus(ch.Name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": channels})
}

// CreateChannel creates a new channel.
func (h *AdminHandler) CreateChannel(c *gin.Context) {
	var rec storage.ChannelRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if rec.Name == "" || rec.BaseURL == "" || rec.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, type, and base_url are required"})
		return
	}

	if err := h.repo.CreateChannel(&rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Trigger hot reload into Data Plane memory and broadcast to cluster
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channel_created")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rec, "message": "Channel created and synchronized to memory successfully"})
}

// UpdateChannel updates an existing channel.
func (h *AdminHandler) UpdateChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var rec storage.ChannelRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rec.ID = id

	if err := h.repo.UpdateChannel(&rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channel_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rec, "message": "Channel updated successfully"})
}

// DeleteChannel removes a channel.
func (h *AdminHandler) DeleteChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.repo.DeleteChannel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channel_deleted")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "Channel deleted successfully"})
}

// TestChannel tests the live connectivity of a channel.
func (h *AdminHandler) TestChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	channels, err := h.repo.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var target *storage.ChannelRecord
	for _, ch := range channels {
		if ch.ID == id {
			target = ch
			break
		}
	}

	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}

	testModel := "gpt-3.5-turbo"
	if len(target.Models) > 0 {
		testModel = target.Models[0]
	}

	chCfg := model.ChannelConfig{
		Name:           target.Name,
		Type:           target.Type,
		BaseURL:        target.BaseURL,
		APIKey:         target.APIKey,
		Models:         target.Models,
		ModelMapping:   target.ModelMapping,
		TimeoutSeconds: 15,
	}

	var prov provider.Provider
	switch target.Type {
	case model.ProviderAnthropic:
		prov = provider.NewAnthropicProvider(nil)
	case model.ProviderGemini:
		prov = provider.NewGeminiProvider(nil)
	default:
		prov = provider.NewOpenAIProvider(nil)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	testReq := &model.ChatCompletionRequest{
		Model: testModel,
		Messages: []model.ChatMessage{
			{Role: "user", Content: "ping"},
		},
	}

	start := time.Now()
	resp, err := prov.ChatComplete(ctx, testReq, &chCfg)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":       1,
			"success":    false,
			"latency_ms": latencyMs,
			"error":      err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"success":    true,
		"latency_ms": latencyMs,
		"response":   resp.Choices[0].Message.GetContentString(),
	})
}

// ListVirtualKeys returns all virtual keys (scoped to current user if non-admin).
func (h *AdminHandler) ListVirtualKeys(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims := claimsVal.(*AdminClaims)
		if claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user != nil {
				keys, err := h.repo.ListVirtualKeysByUser(user.ID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
				return
			}
		}
	}

	keys, err := h.repo.ListVirtualKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": keys})
}

// CreateVirtualKey generates a new virtual API key.
func (h *AdminHandler) CreateVirtualKey(c *gin.Context) {
	var rec storage.VirtualKeyRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims := claimsVal.(*AdminClaims)
		if claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user != nil {
				rec.UserID = user.ID
				rec.GroupName = user.GroupName
			}
		}
	}

	if rec.Key == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		rec.Key = fmt.Sprintf("sk-nano-%s", hex.EncodeToString(b))
	}
	if rec.TenantID == "" {
		rec.TenantID = "default-app"
	}

	if err := h.repo.CreateVirtualKey(&rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "virtual_key_created")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rec, "message": "Virtual Key created successfully"})
}

// UpdateVirtualKey updates an existing virtual key (e.g. status toggle, RPM, tenant, allowed models).
func (h *AdminHandler) UpdateVirtualKey(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	existing, err := h.repo.GetVirtualKey(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "virtual key not found"})
		return
	}

	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims := claimsVal.(*AdminClaims)
		if claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user == nil || existing.UserID != user.ID {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权修改该密钥"})
				return
			}
		}
	}

	var req struct {
		TenantID      *string   `json:"tenant_id"`
		AllowedModels []string  `json:"allowed_models"`
		RPM           *int      `json:"rpm"`
		TPM           *int      `json:"tpm"`
		Budget        *float64  `json:"budget"`
		Status        *string   `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TenantID != nil {
		existing.TenantID = *req.TenantID
	}
	if req.AllowedModels != nil {
		existing.AllowedModels = req.AllowedModels
	}
	if req.RPM != nil {
		existing.RPM = *req.RPM
	}
	if req.TPM != nil {
		existing.TPM = *req.TPM
	}
	if req.Budget != nil {
		existing.Budget = *req.Budget
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}

	if err := h.repo.UpdateVirtualKey(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "virtual_key_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": existing, "message": "Virtual Key updated successfully"})
}

// DeleteVirtualKey removes a virtual key.
func (h *AdminHandler) DeleteVirtualKey(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims := claimsVal.(*AdminClaims)
		if claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user == nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
				return
			}
			existing, err := h.repo.GetVirtualKey(id)
			if err != nil || existing == nil || existing.UserID != user.ID {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权操作该密钥"})
				return
			}
		}
	}

	if err := h.repo.DeleteVirtualKey(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "virtual_key_deleted")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "Virtual Key deleted successfully"})
}

// GetStatsOverview returns dashboard overview metrics.
func (h *AdminHandler) GetStatsOverview(c *gin.Context) {
	stats, err := h.repo.GetStatsOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
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
			Mode:        "Redis Stream (nano:log_stream) 消费组",
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
	smtpMode := "系统内置通知通道 (零配置直接送达)"
	if os.Getenv("SMTP_HOST") != "" {
		smtpMode = fmt.Sprintf("SMTP 邮件中继服务 (%s)", os.Getenv("SMTP_HOST"))
	}
	list = append(list, MiddlewareComponentStatus{
		ID:          "notification",
		Name:        "用户通知与安全验证系统",
		Category:    "通知与认证",
		Status:      "operational",
		Mode:        smtpMode,
		Description: "新用户注册安全验证码发放、密码找回通知、余额告警触达与消息通知保障。",
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

// UpdateModelRouteRequest specifies model-level distribution, mapping, and fallback settings.
type UpdateModelRouteRequest struct {
	Model           string                `json:"model" binding:"required"`
	FallbackModel   string                `json:"fallback_model"`
	ProviderUpdates []ModelProviderUpdate `json:"provider_updates"`
	PromptPrice     *float64              `json:"prompt_price,omitempty"`
	CompletionPrice *float64              `json:"completion_price,omitempty"`
	CacheReadPrice  *float64              `json:"cache_read_price,omitempty"`
	FixedPrice      *float64              `json:"fixed_price,omitempty"`
	Currency        string                `json:"currency,omitempty"`
	OffPeakEnabled  *bool                 `json:"off_peak_enabled,omitempty"`
	OffPeakMode     string                `json:"off_peak_mode,omitempty"`
	OffPeakSlots    string                `json:"off_peak_slots,omitempty"`
	WeekendAllDay   *bool                 `json:"weekend_all_day,omitempty"`
	OffPeakStart    string                `json:"off_peak_start,omitempty"`
	OffPeakEnd      string                `json:"off_peak_end,omitempty"`
	OffPeakDiscount *float64              `json:"off_peak_discount,omitempty"`
}

// ModelProviderUpdate describes changes to a provider's weight, priority, or mapped model.
type ModelProviderUpdate struct {
	ChannelID   int64  `json:"channel_id"`
	Priority    int    `json:"priority"`
	Weight      int    `json:"weight"`
	MappedModel string `json:"mapped_model"`
}

// GetModelRoutes returns enriched model-centric routing topology, weights, and fallback rules.
func (h *AdminHandler) GetModelRoutes(c *gin.Context) {
	routes := h.dispatcher.GetModelRoutes()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": routes})
}

// UpdateModelRoute updates provider weights, priorities, mapped models, and cross-model fallback.
func (h *AdminHandler) UpdateModelRoute(c *gin.Context) {
	var req UpdateModelRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模型标识不能为空"})
		return
	}

	// 1. Update Fallback model
	if req.FallbackModel != "" && req.FallbackModel != req.Model {
		if err := h.repo.SetModelFallback(req.Model, req.FallbackModel, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save model fallback: " + err.Error()})
			return
		}
	} else {
		_ = h.repo.DeleteModelFallback(req.Model)
	}

	// 2. Save model pricing if provided
	if req.PromptPrice != nil || req.CompletionPrice != nil || req.CacheReadPrice != nil || req.FixedPrice != nil {
		currency := req.Currency
		if currency == "" {
			currency = "CNY"
		}
		var pPrice, cPrice, cachePrice, fPrice float64
		if req.PromptPrice != nil {
			pPrice = *req.PromptPrice
		}
		if req.CompletionPrice != nil {
			cPrice = *req.CompletionPrice
		}
		if req.CacheReadPrice != nil {
			cachePrice = *req.CacheReadPrice
		}
		if req.FixedPrice != nil {
			fPrice = *req.FixedPrice
		}
		existing, _ := h.repo.GetModelPrice(req.Model)
		offEnabled := true
		offMode := "deepseek"
		offSlots := ""
		weekendAll := true
		offStart := "00:00"
		offEnd := "08:30"
		offDiscount := 0.5
		if existing != nil {
			offEnabled = existing.OffPeakEnabled
			offMode = existing.OffPeakMode
			offSlots = existing.OffPeakSlots
			weekendAll = existing.WeekendAllDay
			offStart = existing.OffPeakStart
			offEnd = existing.OffPeakEnd
			offDiscount = existing.OffPeakDiscount
		}
		if req.OffPeakEnabled != nil {
			offEnabled = *req.OffPeakEnabled
		}
		if req.OffPeakMode != "" {
			offMode = req.OffPeakMode
		}
		if req.OffPeakSlots != "" {
			offSlots = req.OffPeakSlots
		}
		if req.WeekendAllDay != nil {
			weekendAll = *req.WeekendAllDay
		}
		if req.OffPeakStart != "" {
			offStart = req.OffPeakStart
		}
		if req.OffPeakEnd != "" {
			offEnd = req.OffPeakEnd
		}
		if req.OffPeakDiscount != nil {
			offDiscount = *req.OffPeakDiscount
		}

		if offEnabled && offSlots != "" {
			if err := billing.ValidateSlotsOverlap(offSlots); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}

		_ = h.repo.SaveModelPrice(&storage.ModelPriceRecord{
			Model:           req.Model,
			PromptPrice:     pPrice,
			CompletionPrice: cPrice,
			CacheReadPrice:  cachePrice,
			FixedPrice:      fPrice,
			Currency:        currency,
			OffPeakEnabled:  offEnabled,
			OffPeakMode:     offMode,
			OffPeakSlots:    offSlots,
			WeekendAllDay:   weekendAll,
			OffPeakStart:    offStart,
			OffPeakEnd:      offEnd,
			OffPeakDiscount: offDiscount,
		})
		if billing.GlobalEngine != nil {
			_ = billing.GlobalEngine.ReloadPrices()
		}
	}

	// 3. Build map of desired channels for this model
	desiredProviders := make(map[int64]ModelProviderUpdate)
	for _, pu := range req.ProviderUpdates {
		desiredProviders[pu.ChannelID] = pu
	}

	// 4. Update channels: bind desired channels, unbind removed channels
	allChannels, _ := h.repo.ListChannels()
	for _, ch := range allChannels {
		pu, isDesired := desiredProviders[ch.ID]
		changed := false

		containsModel := false
		for _, m := range ch.Models {
			if m == req.Model {
				containsModel = true
				break
			}
		}

		if isDesired {
			if !containsModel {
				ch.Models = append(ch.Models, req.Model)
				changed = true
			}
			if pu.Priority > 0 && ch.Priority != pu.Priority {
				ch.Priority = pu.Priority
				changed = true
			}
			if pu.Weight > 0 && ch.Weight != pu.Weight {
				ch.Weight = pu.Weight
				changed = true
			}
			if ch.ModelMapping == nil {
				ch.ModelMapping = make(map[string]string)
			}
			if pu.MappedModel != "" && pu.MappedModel != req.Model {
				if ch.ModelMapping[req.Model] != pu.MappedModel {
					ch.ModelMapping[req.Model] = pu.MappedModel
					changed = true
				}
			} else {
				if _, exists := ch.ModelMapping[req.Model]; exists {
					delete(ch.ModelMapping, req.Model)
					changed = true
				}
			}
		} else {
			if containsModel {
				var remaining []string
				for _, m := range ch.Models {
					if m != req.Model {
						remaining = append(remaining, m)
					}
				}
				ch.Models = remaining
				changed = true
			}
			if ch.ModelMapping != nil {
				if _, exists := ch.ModelMapping[req.Model]; exists {
					delete(ch.ModelMapping, req.Model)
					changed = true
				}
			}
		}

		if changed {
			_ = h.repo.UpdateChannel(ch)
		}
	}

	// 5. Atomically sync DB state into Dispatcher and broadcast to Redis cluster
	if err := h.sync.ReloadAndBroadcast(c.Request.Context(), "model_route_updated"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reload data plane: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型多源路由及分发容灾配置已保存并完成热重载"})
}

// DeleteModelRoute removes a model from all upstream channels and fallback definitions.
func (h *AdminHandler) DeleteModelRoute(c *gin.Context) {
	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model name required"})
		return
	}

	allChannels, err := h.repo.ListChannels()
	if err == nil {
		for _, ch := range allChannels {
			changed := false
			var newModels []string
			for _, m := range ch.Models {
				if m == modelName {
					changed = true
				} else {
					newModels = append(newModels, m)
				}
			}
			if changed {
				ch.Models = newModels
			}
			if ch.ModelMapping != nil {
				if _, ok := ch.ModelMapping[modelName]; ok {
					delete(ch.ModelMapping, modelName)
					changed = true
				}
			}
			if changed {
				_ = h.repo.UpdateChannel(ch)
			}
		}
	}

	_ = h.repo.DeleteModelFallback(modelName)

	if err := h.sync.ReloadAndBroadcast(c.Request.Context(), "model_route_deleted"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reload: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型路由及上下游绑定已成功删除"})
}

// GetPricingRates returns all model pricing rates and cache savings benchmark.
func (h *AdminHandler) GetPricingRates(c *gin.Context) {
	rates, err := h.repo.ListModelPrices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pricing rates: " + err.Error()})
		return
	}

	type enrichedPrice struct {
		*storage.ModelPriceRecord
		IsCurrentOffPeak  bool    `json:"is_current_off_peak"`
		EffectiveDiscount float64 `json:"effective_discount"`
	}

	now := time.Now()
	loc, errLoc := time.LoadLocation("Asia/Shanghai")
	if errLoc != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	nowCST := now.In(loc)

	enriched := make([]enrichedPrice, 0, len(rates))
	for _, r := range rates {
		isOff, disc := false, 1.0
		if billing.GlobalEngine != nil {
			isOff, disc = billing.GlobalEngine.IsOffPeak(r, now)
		}
		enriched = append(enriched, enrichedPrice{
			ModelPriceRecord:  r,
			IsCurrentOffPeak:  isOff,
			EffectiveDiscount: disc,
		})
	}

	weekday := nowCST.Weekday()
	isWeekend := (weekday == time.Saturday || weekday == time.Sunday)

	c.JSON(http.StatusOK, gin.H{
		"code":           0,
		"data":           enriched,
		"server_time":    nowCST.Format("15:04:05"),
		"server_weekday": weekday.String(),
		"is_weekend":     isWeekend,
	})
}

// SavePricingRate creates or updates pricing for a model.
func (h *AdminHandler) SavePricingRate(c *gin.Context) {
	var req storage.ModelPriceRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}
	if req.OffPeakEnabled && req.OffPeakSlots != "" {
		if err := billing.ValidateSlotsOverlap(req.OffPeakSlots); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.repo.SaveModelPrice(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save model price: " + err.Error()})
		return
	}

	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "model_price_updated")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "模型计费单价配置已保存并实时生效"})
}

// DeletePricingRate removes pricing for a model (optionally within a group).
func (h *AdminHandler) DeletePricingRate(c *gin.Context) {
	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model name required"})
		return
	}
	groupName := strings.TrimSpace(c.Query("group"))
	var err error
	if groupName != "" {
		err = h.repo.DeleteModelPriceWithGroup(modelName, groupName)
	} else {
		err = h.repo.DeleteModelPrice(modelName)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete model price: " + err.Error()})
		return
	}

	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "model_price_deleted")

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除该模型计费规则"})
}

// ProbeModelRoute tests connectivity and latency for all providers of a specific model in parallel.
func (h *AdminHandler) ProbeModelRoute(c *gin.Context) {
	var req struct {
		Model string `json:"model" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	channels := h.dispatcher.GetChannelsForModel(req.Model)
	type ProbeResultItem struct {
		ChannelID int64  `json:"channel_id"`
		Name      string `json:"name"`
		LatencyMs int64  `json:"latency_ms"`
		Status    string `json:"status"`
		Error     string `json:"error,omitempty"`
	}

	results := make([]ProbeResultItem, len(channels))
	var wg stdSync.WaitGroup
	client := &http.Client{Timeout: 4 * time.Second}

	for i, ch := range channels {
		wg.Add(1)
		go func(idx int, c model.ChannelConfig) {
			defer wg.Done()
			start := time.Now()
			probeURL := strings.TrimRight(c.BaseURL, "/") + "/models"
			httpReq, err := http.NewRequest("GET", probeURL, nil)
			if err != nil {
				results[idx] = ProbeResultItem{
					ChannelID: c.ID,
					Name:      c.Name,
					Status:    "error",
					Error:     err.Error(),
				}
				return
			}
			if c.APIKey != "" && c.APIKey != "none" {
				httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
				httpReq.Header.Set("x-api-key", c.APIKey)
			}
			resp, err := client.Do(httpReq)
			lat := time.Since(start).Milliseconds()
			if err != nil {
				results[idx] = ProbeResultItem{
					ChannelID: c.ID,
					Name:      c.Name,
					LatencyMs: lat,
					Status:    "unreachable",
					Error:     err.Error(),
				}
				return
			}
			defer resp.Body.Close()
			status := "healthy"
			if resp.StatusCode >= 400 && resp.StatusCode != 404 && resp.StatusCode != 405 {
				status = fmt.Sprintf("status_%d", resp.StatusCode)
			}
			results[idx] = ProbeResultItem{
				ChannelID: c.ID,
				Name:      c.Name,
				LatencyMs: lat,
				Status:    status,
			}
		}(i, *ch)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": results})
}

// ListLogs returns audit usage logs with optional time range, chat ID, and session filtering.
func (h *AdminHandler) ListLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	offsetStr := c.DefaultQuery("offset", "0")
	offset, _ := strconv.Atoi(offsetStr)

	filter := storage.LogFilter{
		Limit:     limit,
		Offset:    offset,
		StartTime: c.Query("start_time"),
		EndTime:   c.Query("end_time"),
		TraceID:   c.Query("trace_id"),
		ChatID:    c.Query("chat_id"),
		SessionID: c.Query("session_id"),
		Model:     c.Query("model"),
		TenantID:  c.Query("tenant_id"),
	}

	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims := claimsVal.(*AdminClaims)
		if claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user != nil {
				userKeys, _ := h.repo.ListVirtualKeysByUser(user.ID)
				if len(userKeys) == 0 {
					c.JSON(http.StatusOK, gin.H{"code": 0, "data": []*storage.UsageLogRecord{}})
					return
				}
				userKeyMap := make(map[string]bool)
				for _, k := range userKeys {
					userKeyMap[k.Key] = true
				}
				logs, err := h.repo.ListUsageLogsWithFilter(filter)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				userFiltered := make([]*storage.UsageLogRecord, 0)
				for _, l := range logs {
					if userKeyMap[l.VirtualKey] {
						userFiltered = append(userFiltered, l)
					}
				}
				c.JSON(http.StatusOK, gin.H{"code": 0, "data": userFiltered})
				return
			}
		}
	}

	logs, err := h.repo.ListUsageLogsWithFilter(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = make([]*storage.UsageLogRecord, 0)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": logs})
}

// GetRepo returns the underlying storage repository.
func (h *AdminHandler) GetRepo() *storage.Repository {
	if h == nil {
		return nil
	}
	return h.repo
}

// ListSkills returns all Agent Skills with their current enabled states.
func (h *AdminHandler) ListSkills(c *gin.Context) {
	skills, err := h.repo.ListSkills()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "获取技能列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": skills})
}

// ToggleSkillRequest defines toggle payload.
type ToggleSkillRequest struct {
	Enabled *bool `json:"enabled"`
}

// ToggleSkill enables or disables an Agent Skill on-demand.
func (h *AdminHandler) ToggleSkill(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "技能 ID 不能为空"})
		return
	}

	var req ToggleSkillRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.Enabled != nil {
		if err := h.repo.SetSkillEnabled(id, *req.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新技能状态失败: " + err.Error()})
			return
		}
	} else {
		current := h.repo.IsSkillEnabled(id)
		if err := h.repo.SetSkillEnabled(id, !current); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "切换技能状态失败: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "技能状态更新成功", "enabled": h.repo.IsSkillEnabled(id)})
}

// GetMCPSettings returns the master MCP enable/disable switch and config.
func (h *AdminHandler) GetMCPSettings(c *gin.Context) {
	enabled := h.repo.GetSetting("mcp_enabled", "true") == "true"
	skills, _ := h.repo.ListSkills()
	enabledSkillsCount := 0
	totalToolsCount := 0
	for _, s := range skills {
		if s.Enabled {
			enabledSkillsCount++
			totalToolsCount += len(s.Tools)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"mcp_enabled":          enabled,
			"enabled_skills_count": enabledSkillsCount,
			"total_skills_count":   len(skills),
			"active_tools_count":   totalToolsCount,
			"sse_endpoint":         "/mcp/sse",
			"messages_endpoint":    "/mcp/messages",
		},
	})
}

// UpdateMCPSettingsRequest defines update payload.
type UpdateMCPSettingsRequest struct {
	MCPEnabled bool `json:"mcp_enabled"`
}

// UpdateMCPSettings sets the master MCP enable/disable switch.
func (h *AdminHandler) UpdateMCPSettings(c *gin.Context) {
	var req UpdateMCPSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请求参数不合法"})
		return
	}

	val := "false"
	if req.MCPEnabled {
		val = "true"
	}
	if err := h.repo.SetSetting("mcp_enabled", val); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新 MCP 配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务状态已更新", "mcp_enabled": req.MCPEnabled})
}

// DeleteLog deletes a single log by ID (admin only).
func (h *AdminHandler) DeleteLog(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "权限不足，仅超级管理员可删除审计日志"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log id"})
		return
	}
	if err := h.repo.DeleteUsageLog(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除该条调用日志"})
}

// BatchDeleteLogs deletes multiple logs by IDs (admin only).
func (h *AdminHandler) BatchDeleteLogs(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "权限不足，仅超级管理员可删除审计日志"})
		return
	}

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的日志 ID 列表"})
		return
	}
	n, err := h.repo.BatchDeleteUsageLogs(req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 条调用日志", n), "deleted_count": n})
}

// ClearLogs clears all audit logs (admin only).
func (h *AdminHandler) ClearLogs(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "权限不足，仅超级管理员可清空审计日志"})
		return
	}

	if err := h.repo.ClearAllUsageLogs(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已清空所有调用日志"})
}

// BatchDeleteChannels deletes multiple channels.
func (h *AdminHandler) BatchDeleteChannels(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效渠道 ID 列表"})
		return
	}
	n, err := h.repo.BatchDeleteChannels(req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channels_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个服务商渠道", n), "deleted_count": n})
}

// BatchStatusChannels toggles status for multiple channels.
func (h *AdminHandler) BatchStatusChannels(c *gin.Context) {
	var req struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"` // "active" or "disabled"
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效请求参数"})
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		req.Status = "active"
	}
	n, err := h.repo.BatchUpdateChannelStatus(req.IDs, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "channels_batch_status_updated")
	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个服务商渠道", action, n)})
}

// BatchDeleteVirtualKeys deletes multiple virtual keys.
func (h *AdminHandler) BatchDeleteVirtualKeys(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效密钥 ID 列表"})
		return
	}

	targetIDs := req.IDs
	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims, ok := claimsVal.(*AdminClaims)
		if ok && claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user == nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
				return
			}
			userKeys, err := h.repo.ListVirtualKeysByUser(user.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			allowedMap := make(map[int64]bool)
			for _, k := range userKeys {
				allowedMap[k.ID] = true
			}
			var filteredIDs []int64
			for _, id := range req.IDs {
				if allowedMap[id] {
					filteredIDs = append(filteredIDs, id)
				}
			}
			if len(filteredIDs) == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权操作所选密钥"})
				return
			}
			targetIDs = filteredIDs
		}
	}

	n, err := h.repo.BatchDeleteVirtualKeys(targetIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "keys_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量注销 %d 个 API 访问密钥", n), "deleted_count": n})
}

// BatchStatusVirtualKeys toggles status for multiple virtual keys.
func (h *AdminHandler) BatchStatusVirtualKeys(c *gin.Context) {
	var req struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"` // "active" or "disabled"
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效请求参数"})
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		req.Status = "active"
	}

	targetIDs := req.IDs
	claimsVal, exists := c.Get("admin_claims")
	if exists {
		claims, ok := claimsVal.(*AdminClaims)
		if ok && claims.Role != "admin" {
			user, _ := h.repo.GetUserByUsername(claims.Username)
			if user == nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
				return
			}
			userKeys, err := h.repo.ListVirtualKeysByUser(user.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			allowedMap := make(map[int64]bool)
			for _, k := range userKeys {
				allowedMap[k.ID] = true
			}
			var filteredIDs []int64
			for _, id := range req.IDs {
				if allowedMap[id] {
					filteredIDs = append(filteredIDs, id)
				}
			}
			if len(filteredIDs) == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权操作所选密钥"})
				return
			}
			targetIDs = filteredIDs
		}
	}

	n, err := h.repo.BatchUpdateVirtualKeyStatus(targetIDs, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "keys_batch_status_updated")
	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个 API 访问密钥", action, n)})
}

// BatchDeleteModelRoutes deletes routes for multiple models.
func (h *AdminHandler) BatchDeleteModelRoutes(c *gin.Context) {
	var req struct {
		Models []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Models) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供待删除模型列表"})
		return
	}

	modelSet := make(map[string]bool)
	for _, m := range req.Models {
		t := strings.TrimSpace(m)
		if t != "" {
			modelSet[t] = true
		}
	}

	allChannels, err := h.repo.ListChannels()
	if err == nil {
		for _, ch := range allChannels {
			changed := false
			var newModels []string
			for _, m := range ch.Models {
				if modelSet[m] {
					changed = true
				} else {
					newModels = append(newModels, m)
				}
			}
			if changed {
				ch.Models = newModels
			}
			if ch.ModelMapping != nil {
				for m := range modelSet {
					if _, ok := ch.ModelMapping[m]; ok {
						delete(ch.ModelMapping, m)
						changed = true
					}
				}
			}
			if changed {
				_ = h.repo.UpdateChannel(ch)
			}
		}
	}

	for m := range modelSet {
		_ = h.repo.DeleteModelFallback(m)
	}

	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "models_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个模型路由及其服务商绑定", len(modelSet))})
}

// BatchDeletePricingRates deletes pricing for multiple models.
func (h *AdminHandler) BatchDeletePricingRates(c *gin.Context) {
	var req struct {
		Models []string                `json:"models"`
		Items  []storage.ModelPriceKey `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	var n int64
	var err error
	if len(req.Items) > 0 {
		n, err = h.repo.BatchDeleteModelPriceKeys(req.Items)
	} else if len(req.Models) > 0 {
		n, err = h.repo.BatchDeleteModelPrices(req.Models)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供待删除定价模型列表"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}
	_ = h.sync.ReloadAndBroadcast(c.Request.Context(), "pricing_batch_deleted")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个模型计费费率", n)})
}
