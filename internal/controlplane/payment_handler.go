package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

type CreateStripeSessionRequest struct {
	Amount float64 `json:"amount" binding:"required"`
}

func validCheckoutKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (h *AdminHandler) CreateStripeRechargeSession(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}
	var req CreateStripeSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil || !storage.ValidRechargeAmount(req.Amount) {
		c.JSON(400, gin.H{"error": "充值金额须为 0.01 至 999999.99 元，最多两位小数"})
		return
	}
	cfg, err := loadStripeConfig()
	if err != nil || !cfg.enabled() {
		c.JSON(503, gin.H{"error": "线上收款通道尚未配置或配置无效，请使用兑换券或联系管理员"})
		return
	}
	if err := h.repo.BindPaymentMode(cfg.Mode); err != nil {
		c.JSON(503, gin.H{"error": "支付环境与数据库不一致，请联系管理员"})
		return
	}
	if req.Amount < cfg.MinAmount {
		c.JSON(400, gin.H{"error": "充值金额低于当前收款通道最低金额", "min_amount": cfg.MinAmount})
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if !validCheckoutKey(key) {
		c.JSON(400, gin.H{"error": "请提供 8 至 128 位的 Idempotency-Key（字母、数字、短横线或下划线）"})
		return
	}
	u, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || u == nil || u.Status != "active" {
		c.JSON(403, gin.H{"error": "当前账号不可充值"})
		return
	}
	if u.Balance+req.Amount > storage.MaxUserBalance {
		c.JSON(400, gin.H{"error": "充值后余额将超过上限"})
		return
	}
	token, err := GenerateAPIKey()
	if err != nil {
		c.JSON(500, gin.H{"error": "生成订单号失败"})
		return
	}
	number := "REC" + strings.TrimPrefix(token, "sk-")
	o, err := h.repo.CreateRechargeIntent(&storage.RechargeOrderRecord{OrderNo: number, Username: claims.Username, Amount: math.Round(req.Amount*100) / 100, Currency: "CNY", IdempotencyKey: key, PaymentMode: cfg.Mode,
		SuccessURL: cfg.returnURL(number, "success"), CancelURL: cfg.returnURL(number, "cancel"), ExpiresAt: time.Now().Add(23 * time.Hour).Unix()})
	if errors.Is(err, storage.ErrPaymentConflict) {
		c.JSON(409, gin.H{"error": "该支付请求已绑定其他金额，请重新确认充值金额", "reason": "idempotency_conflict"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "保存充值订单失败，未发起支付请求"})
		return
	}
	number = o.OrderNo
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	o, err = h.syncStripeOrder(ctx, number, cfg, true, "")
	if err != nil {
		telemetry.ScopedLogger("payment").Warn("checkout creation or recovery pending", "order_no", number, "error", err.Error())
		c.Header("Retry-After", "5")
		c.JSON(503, gin.H{"error": "支付服务暂不可用，请稍后重试同一请求，避免重复支付", "order_no": number, "retryable": true})
		return
	}
	result := gin.H{"code": 0, "order_no": o.OrderNo, "status": o.Status}
	if o.Status == "pending" && o.CheckoutURL != "" && o.ExpiresAt > time.Now().Unix() {
		result["checkout_url"] = o.CheckoutURL
	}
	c.JSON(200, result)
}

func (h *AdminHandler) GetUserRechargeOrder(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}
	o, err := h.repo.GetRechargeOrder(c.Param("order_no"))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(500, gin.H{"error": "查询订单失败"})
		return
	}
	if o == nil || o.Username != claims.Username {
		c.JSON(404, gin.H{"error": "订单不存在"})
		return
	}
	if cfg, err := loadStripeConfig(); err == nil && cfg.enabled() && o.PaymentMode == cfg.Mode {
		if err := h.repo.BindPaymentMode(cfg.Mode); err != nil {
			c.JSON(503, gin.H{"error": "支付环境与数据库不一致"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		if updated, err := h.syncStripeOrder(ctx, o.OrderNo, cfg, false, ""); err == nil {
			o = updated
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": o})
}
