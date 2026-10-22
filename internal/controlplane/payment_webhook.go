package controlplane

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
			return fmt.Errorf("Webhook 时间戳超出允许容忍时间窗口 (可能为重放攻击)")
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
		return fmt.Errorf("Webhook 签名验证不通过")
	}

	return nil
}

// StripeWebhook processes Stripe payment webhook events.
func (h *AdminHandler) StripeWebhook(c *gin.Context) {
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	sigHeader := c.GetHeader("Stripe-Signature")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求体"})
		return
	}

	// In production, refuse to process webhooks if secret is not configured
	if gin.Mode() == gin.ReleaseMode && webhookSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生产环境未配置 STRIPE_WEBHOOK_SECRET，拒绝接收未经签名的 Webhook 请求"})
		return
	}

	// Verify webhook signature in production or whenever secret/signature header is present
	if webhookSecret != "" || sigHeader != "" || gin.Mode() != gin.TestMode {
		if err := VerifyStripeWebhookSignature(body, sigHeader, webhookSecret, 5*time.Minute); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Stripe 签名校验失败: " + err.Error()})
			return
		}
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string `json:"id"`
				ClientReferenceID string `json:"client_reference_id"`
				PaymentStatus     string `json:"payment_status"`
				AmountTotal       int64  `json:"amount_total"`
				Currency          string `json:"currency"`
			} `json:"object"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析事件失败"})
		return
	}

	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.expired", "checkout.session.async_payment_failed":
		obj := event.Data.Object
		if (event.Type == "checkout.session.completed" || event.Type == "checkout.session.async_payment_succeeded") && obj.PaymentStatus != "paid" {
			c.JSON(http.StatusOK, gin.H{"received": true, "status": event.Data.Object.PaymentStatus})
			return
		}
		order, err := h.repo.GetRechargeOrder(obj.ClientReferenceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "订单暂不可用，请重试"})
			return
		}
		if order.Channel != "stripe" || obj.ID == "" || order.StripeSessionID != obj.ID || !strings.EqualFold(order.Currency, obj.Currency) || int64(math.Round(order.Amount*100)) != obj.AmountTotal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "支付事件与订单的会话、金额或币种不一致"})
			return
		}
		switch event.Type {
		case "checkout.session.expired":
			err = h.repo.CloseRechargeOrder(order.OrderNo, "expired")
		case "checkout.session.async_payment_failed":
			err = h.repo.CloseRechargeOrder(order.OrderNo, "failed")
		default:
			_, err = h.repo.CompleteRechargeOrder(order.OrderNo)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "订单处理失败: " + err.Error()})
			return
		}
		if obj.PaymentStatus == "paid" {
			h.syncDataPlane()
		}
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
