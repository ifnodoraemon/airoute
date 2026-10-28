package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// CreateStripeSessionRequest defines top-up checkout parameters.
type CreateStripeSessionRequest struct {
	Amount   float64 `json:"amount" binding:"required"`
	Currency string  `json:"currency"`
}

// CreateStripeRechargeSession initiates a Stripe Checkout Session or returns checkout configuration.
func (h *AdminHandler) CreateStripeRechargeSession(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	var req CreateStripeSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效的充值金额"})
		return
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "CNY"
	}
	// Wallet balances are denominated in CNY. Normalize once to the minor
	// unit used by checkout, so the charged amount and credited amount agree.
	if currency != "CNY" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "钱包充值仅支持人民币 CNY"})
		return
	}
	cents := math.Round(req.Amount * 100)
	if !storage.ValidRechargeAmount(req.Amount) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "充值金额须为 0.01 至 999999.99 元，最多保留两位小数"})
		return
	}
	req.Amount = cents / 100

	orderToken, err := GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订单号失败"})
		return
	}
	orderNo := "REC" + strings.TrimPrefix(orderToken, "sk-")

	order := &storage.RechargeOrderRecord{
		OrderNo:  orderNo,
		Username: claims.Username,
		Amount:   req.Amount,
		Currency: currency,
		Channel:  "stripe",
		Status:   "pending",
	}

	baseURL := ResolvePublicBaseURL(c)
	stripeKey := os.Getenv("STRIPE_API_KEY")
	if strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "error": "支付到账通知尚未配置，请使用兑换券或联系管理员"})
		return
	}
	successURL := os.Getenv("STRIPE_SUCCESS_URL")
	if successURL == "" {
		successURL = fmt.Sprintf("%s/app/?tab=wallet&recharge=success&order_no=%s", baseURL, orderNo)
	}

	if stripeKey != "" {
		// One documented name for the API base. STRIPE_BASE_URL used to be an
		// undocumented second spelling that appeared in no manifest or manual, so a
		// stale value in the container environment could silently redirect the
		// checkout call to a different endpoint.
		stripeBase := strings.TrimSpace(os.Getenv("STRIPE_API_BASE"))
		if stripeBase == "" {
			stripeBase = "https://api.stripe.com"
		}
		stripeEndpoint := fmt.Sprintf("%s/v1/checkout/sessions", strings.TrimRight(stripeBase, "/"))

		data := url.Values{}
		data.Set("success_url", successURL)
		data.Set("cancel_url", fmt.Sprintf("%s/app/?tab=wallet&recharge=cancel&order_no=%s", baseURL, orderNo))
		data.Set("payment_method_types[0]", "card")
		data.Set("mode", "payment")
		data.Set("adaptive_pricing[enabled]", "false")
		data.Set("client_reference_id", orderNo)
		data.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
		data.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(int64(cents), 10))
		data.Set("line_items[0][price_data][product_data][name]", fmt.Sprintf("Airoute 钱包充值 (¥%.2f)", req.Amount))
		data.Set("line_items[0][quantity]", "1")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		httpReq, err := http.NewRequestWithContext(ctx, "POST", stripeEndpoint, strings.NewReader(data.Encode()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "构造支付请求失败，请稍后重试"})
			return
		}
		httpReq.Header.Set("Authorization", "Bearer "+stripeKey)
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"code":  502,
				"error": fmt.Sprintf("支付服务通信异常: %v。线上充值暂不可用，请使用兑换券充值或稍后重试。", err),
			})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"code":  502,
				"error": "读取支付服务响应失败。线上充值暂不可用，请使用兑换券充值或稍后重试。",
			})
			return
		}

		if resp.StatusCode != http.StatusOK {
			var stripeErr struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(body, &stripeErr)
			errMsg := stripeErr.Error.Message
			if errMsg == "" {
				errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			c.JSON(http.StatusBadGateway, gin.H{
				"code":  502,
				"error": fmt.Sprintf("支付服务通道返回错误 (%s)。线上充值暂不可用，请使用兑换券充值或联系管理员。", errMsg),
			})
			return
		}

		var stripeRes struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &stripeRes); err != nil || stripeRes.URL == "" || stripeRes.ID == "" {
			c.JSON(http.StatusBadGateway, gin.H{
				"code":  502,
				"error": "支付服务响应数据解析失败。线上充值暂不可用，请使用兑换券充值或稍后重试。",
			})
			return
		}

		order.StripeSessionID = stripeRes.ID
		if err := h.repo.CreateRechargeOrder(order); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":  500,
				"error": "创建充值订单记录失败，支付流程已终止。未产生有效账单且未扣款，请使用兑换券充值或联系管理员。",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":         0,
			"order_no":     orderNo,
			"checkout_url": stripeRes.URL,
			"session_id":   stripeRes.ID,
		})
		return
	}

	// Online payment gateway is not configured: strictly reject, no sandbox fallback.
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"code":  503,
		"error": "系统尚未配置线上收款通道。当前不支持在线支付，请使用额度兑换券进行充值或联系管理员。",
	})
}

// GetUserRechargeOrder polls the authenticated customer's own checkout only.
func (h *AdminHandler) GetUserRechargeOrder(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}
	order, err := h.repo.GetRechargeOrder(c.Param("order_no"))
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询订单失败"})
		return
	}
	if order == nil || order.Username != claims.Username {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": order})
}

// SandboxRecharge allows instant wallet recharge for direct/fast checkout (Admin/Dev/Test only).
func (h *AdminHandler) SandboxRecharge(c *gin.Context) {
	claimsVal, exists := c.Get("admin_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "未登录或登录凭证已失效"})
		return
	}
	claims, ok := claimsVal.(*AdminClaims)
	if !ok || claims == nil || claims.Username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "error": "无效的认证凭证"})
		return
	}

	isTestOrDev := gin.Mode() == gin.TestMode || strings.HasSuffix(os.Args[0], ".test") || strings.EqualFold(os.Getenv("ENABLE_SANDBOX_RECHARGE"), "true")
	if !isTestOrDev {
		c.JSON(http.StatusForbidden, gin.H{
			"code":  403,
			"error": "充值沙箱与模拟支付已停用。系统不支持模拟充值，请使用额度兑换券进行充值。",
		})
		return
	}

	username := claims.Username

	var req struct {
		OrderNo string  `json:"order_no"`
		Amount  float64 `json:"amount"`
	}
	_ = c.ShouldBindJSON(&req)

	if req.OrderNo == "" {
		req.OrderNo = c.Query("order_no")
	}
	if req.Amount <= 0 {
		amtStr := c.Query("amount")
		if a, err := strconv.ParseFloat(amtStr, 64); err == nil && a > 0 {
			req.Amount = a
		}
	}
	if req.Amount <= 0 {
		req.Amount = 50.0
	}

	if req.OrderNo == "" {
		req.OrderNo = fmt.Sprintf("SANDBOX%d", time.Now().UnixMicro()%1000000)
	}

	// Create or complete order
	order := &storage.RechargeOrderRecord{
		OrderNo:         req.OrderNo,
		Username:        username,
		Amount:          req.Amount,
		Currency:        "CNY",
		Channel:         "sandbox",
		StripeSessionID: "sandbox_" + req.OrderNo,
		Status:          "pending",
	}
	_ = h.repo.CreateRechargeOrder(order)
	completed, err := h.repo.CompleteRechargeOrder(req.OrderNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "充值入账失败: " + err.Error()})
		return
	}

	u, _ := h.repo.GetUserByUsername(username)
	var newBal float64
	if u != nil {
		newBal = u.Balance
	}
	// Credit the data-plane snapshot immediately; the wallet check runs against
	// that snapshot on every inference request.
	h.syncDataPlane()

	c.JSON(http.StatusOK, gin.H{
		"code":        0,
		"message":     fmt.Sprintf("充值成功！已为用户 [%s] 入账 ¥%.2f", username, completed.Amount),
		"new_balance": newBal,
		"order_no":    req.OrderNo,
	})
}
