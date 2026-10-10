package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListRedemptions lists redemption gift codes.
func (h *AdminHandler) ListRedemptions(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可查看兑换码")
	if !ok {
		return
	}

	codes, err := h.repo.ListRedemptionCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "查询兑换码列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": codes})
}

// GenerateRedemptionsRequest defines batch redemption code generation.
type GenerateRedemptionsRequest struct {
	Count  int     `json:"count"`
	Amount float64 `json:"amount" binding:"required"`
	Name   string  `json:"name"`
}

// GenerateRedemptions batch creates redemption gift codes.
func (h *AdminHandler) GenerateRedemptions(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可生成兑换码")
	if !ok {
		return
	}

	var req GenerateRedemptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效的充值金额"})
		return
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		count = 100
	}

	name := req.Name
	if name == "" {
		name = fmt.Sprintf("额度兑换卡 ¥%.2f", req.Amount)
	}

	createdCodes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		codeBytes := make([]byte, 8)
		_, _ = rand.Read(codeBytes)
		code := fmt.Sprintf("CARD-%s-%s", strings.ToUpper(hex.EncodeToString(codeBytes[:4])), strings.ToUpper(hex.EncodeToString(codeBytes[4:])))

		rec := &storage.RedemptionCodeRecord{
			Code:   code,
			Name:   name,
			Amount: req.Amount,
			Status: "active",
		}
		if err := h.repo.CreateRedemptionCode(rec); err == nil {
			createdCodes = append(createdCodes, code)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("成功生成 %d 张额度兑换卡", len(createdCodes)),
		"data":    createdCodes,
	})
}

// DeleteRedemption removes a redemption code.
func (h *AdminHandler) DeleteRedemption(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可删除兑换码")
	if !ok {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的兑换码ID"})
		return
	}

	if err := h.repo.DeleteRedemptionCode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除兑换码失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "兑换码已删除"})
}

// RedeemRequest defines gift card redemption payload.
type RedeemRequest struct {
	Code string `json:"code" binding:"required"`
}

// RedeemWalletCode redeems a gift card code and adds balance to the current user.
func (h *AdminHandler) RedeemWalletCode(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	var req RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请输入兑换码"})
		return
	}

	rec, err := h.repo.RedeemCode(req.Code, claims.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}

	user, _ := h.repo.GetUserByUsername(claims.Username)
	var newBal float64
	if user != nil {
		newBal = user.Balance
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("恭喜！成功兑换 [%s]，已充值 ¥%.2f 到您的账户", rec.Name, rec.Amount),
		"data": gin.H{
			"amount":      rec.Amount,
			"new_balance": newBal,
		},
	})
}
