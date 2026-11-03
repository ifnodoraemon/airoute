package controlplane

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// VerifyStripeWebhookSignature validates Stripe webhook HMAC-SHA256 signature against webhook secret.
func VerifyStripeWebhookSignature(payload []byte, sigHeader, secret string, tolerance time.Duration) error {
	if secret == "" {
		return fmt.Errorf("STRIPE_WEBHOOK_SECRET 未配置")
	}
	if sigHeader == "" {
		return fmt.Errorf("缺少 Stripe-Signature 请求头")
	}

	var timestampStr string
	var signatures []string

	pairs := strings.Split(sigHeader, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			switch parts[0] {
			case "t":
				timestampStr = parts[1]
			case "v1":
				signatures = append(signatures, parts[1])
			}
		}
	}

	if timestampStr == "" || len(signatures) == 0 {
		return fmt.Errorf("Stripe-Signature 请求头格式无效")
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("无效的时间戳: %w", err)
	}

	if tolerance > 0 {
		now := time.Now().Unix()
		diff := now - ts
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(tolerance.Seconds()) {
			return fmt.Errorf("webhook 时间戳超出允许容忍时间窗口 (可能为重放攻击)")
		}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestampStr))
	mac.Write([]byte("."))
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	matched := false
	for _, sig := range signatures {
		sigBytes, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(sigBytes, expectedSig) {
			matched = true
			break
		}
	}

	if !matched {
		return fmt.Errorf("webhook 签名验证不通过")
	}

	return nil
}

func handledStripeEvent(kind string) bool {
	switch kind {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed", "checkout.session.expired",
		"refund.created", "refund.updated", "refund.failed", "charge.refunded",
		"charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed", "charge.dispute.funds_withdrawn", "charge.dispute.funds_reinstated":
		return true
	}
	return false
}

// Notifications select which object to reconcile. Their amount/status snapshot
// never directly grants or removes wallet credit: current Stripe state does.
func (h *AdminHandler) StripeWebhook(c *gin.Context) {
	cfg, err := loadStripeConfig()
	if err != nil || !cfg.enabled() {
		c.JSON(503, gin.H{"error": "支付通知服务未配置或配置无效"})
		return
	}
	if err := h.repo.BindPaymentMode(cfg.Mode); err != nil {
		c.JSON(503, gin.H{"error": "支付环境与数据库不一致"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024))
	if err != nil {
		c.JSON(400, gin.H{"error": "无法读取支付通知"})
		return
	}
	if err := VerifyStripeWebhookSignature(body, c.GetHeader("Stripe-Signature"), cfg.Secret, 5*time.Minute); err != nil {
		c.JSON(400, gin.H{"error": "Stripe 签名校验失败"})
		return
	}
	var event struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Version  string `json:"api_version"`
		LiveMode *bool  `json:"livemode"`
		Data     struct {
			Object struct {
				ID string `json:"id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(400, gin.H{"error": "支付事件格式无效"})
		return
	}
	if !handledStripeEvent(event.Type) {
		c.JSON(200, gin.H{"received": true})
		return
	}
	if event.ID == "" || event.Data.Object.ID == "" || event.LiveMode == nil || *event.LiveMode != (cfg.Mode == "live") || event.Version != StripeAPIVersion {
		c.JSON(400, gin.H{"error": "支付事件的标识、环境或 API 版本不匹配"})
		return
	}
	e := storage.StripeEventRecord{ID: event.ID, Type: event.Type, ObjectID: event.Data.Object.ID, Mode: cfg.Mode}
	done, err := h.repo.SaveStripeEvent(e)
	if err != nil {
		c.JSON(503, gin.H{"error": "保存支付通知失败，请重试"})
		return
	}
	if done {
		c.JSON(200, gin.H{"received": true})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	err = h.processStripeEvent(ctx, e, cfg)
	if finishErr := h.repo.FinishStripeEvent(e.ID, err); finishErr != nil {
		c.JSON(503, gin.H{"error": "记录支付处理结果失败，请重试"})
		return
	}
	if err != nil {
		telemetry.ScopedLogger("payment").Warn("Stripe notification queued for recovery", "event_id", e.ID, "error", err.Error())
		c.JSON(503, gin.H{"error": "支付通知已保存，等待重新核验"})
		return
	}
	c.JSON(200, gin.H{"received": true})
}
